// Encoding: UTF-8

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
	"github.com/imdario/mergo"
	"gopkg.in/yaml.v3"
)

type StatusConfig struct {
	Bind     string     `yaml:"bind"`
	Commands []*Command `yaml:"commands"`
	Services []*Service `yaml:"services"`
	Proxies  []*Proxy   `yaml:"proxies"`
	Requests []*Request `yaml:"requests"`
}

type GlobalStatus struct {
	Healthy  bool
	Services []SvcStatus     `json:",omitempty"`
	Commands []CmdStatus     `json:",omitempty"`
	Requests []RequestStatus `json:",omitempty"`
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	dcfg := filepath.Join(dir, "go-healthz.yml")
	cfgPath := flag.String("config", dcfg, "Path to configuration file")
	flag.Parse()

	if versionFlag {
		showVersion()
		os.Exit(0)
	}

	if runtime.GOOS == "windows" {
		RunWindows(*cfgPath)
	} else {
		Run(*cfgPath)
	}
}

func Run(cfgPath string) {
	cfgFile, err := os.Open(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	cfgBytes, err := ioutil.ReadAll(cfgFile)
	if err != nil {
		log.Fatal(err)
	}
	cfgFile.Close()

	var cfg StatusConfig
	if err = yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		log.Fatal(err)
	}

	// Enforced Organization Configuration
	if orgPath, ok := os.LookupEnv("GOHEALTHZ_ORG_CONFIG"); ok {
		orgFile, err := os.Open(orgPath)
		if err != nil {
			log.Fatal(err)
		}
		orgBytes, err := ioutil.ReadAll(orgFile)
		if err != nil {
			log.Fatal(err)
		}
		orgFile.Close()
		var orgCfg StatusConfig
		if err = yaml.Unmarshal(orgBytes, &orgCfg); err != nil {
			log.Fatal(err)
		}
		if err = mergo.Merge(&cfg, &orgCfg, mergo.WithAppendSlice); err != nil {
			panic(err)
		}
		// Walk the Configuration, and unique the Slices (Organization Wins)
		cfg.Unique()
	}

	r := mux.NewRouter()

	for _, _svc := range cfg.Services {
		svc := _svc
		r.HandleFunc("/service/"+svc.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := svc.Status()
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		}).Methods("GET")
	}

	for _, _cmd := range cfg.Commands {
		cmd := _cmd
		r.HandleFunc("/command/"+cmd.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := cmd.Status()
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		}).Methods("GET")
	}

	for _, _req := range cfg.Requests {
		req := _req
		r.HandleFunc("/request/"+req.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := req.Status()
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		}).Methods("GET")
	}

	// Ignore Favicon Requests (Browser)
	r.HandleFunc("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) {})

	r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true

		var wg sync.WaitGroup

		//
		// Services
		//
		global.Services = make([]SvcStatus, len(cfg.Services))
		for i, svc := range cfg.Services {
			wg.Add(1)
			go func(i int, svc *Service) {
				defer wg.Done()
				status, _ := svc.Status()
				if !status.Healthy {
					global.Healthy = false
				}
				global.Services[i] = status
			}(i, svc)
		}

		//
		// Commands
		//
		global.Commands = make([]CmdStatus, len(cfg.Commands))
		for i, cmd := range cfg.Commands {
			wg.Add(1)
			go func(i int, cmd *Command) {
				defer wg.Done()
				status, _ := cmd.Status()
				if !status.Healthy {
					global.Healthy = false
				}
				global.Commands[i] = status
			}(i, cmd)
		}

		//
		// Requests
		//
		global.Requests = make([]RequestStatus, len(cfg.Requests))
		for i, req := range cfg.Requests {
			wg.Add(1)
			go func(i int, req *Request) {
				defer wg.Done()
				status, _ := req.Status()
				if !status.Healthy {
					global.Healthy = false
				}
				global.Requests[i] = status
			}(i, req)
		}

		// Waiters
		wg.Wait()

		if global.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(global)
	}).Methods("GET")

	// Local Reverse Proxy
	for _, proxy := range cfg.Proxies {
		p := httputil.NewSingleHostReverseProxy(proxy.URL())
		r.HandleFunc("/"+proxy.Name+"/{rest:.*}", proxy.Handler(p)).Methods(proxy.Methods...)
	}

	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0:8080"
	}
	log.Infoln("Go-Healthz listening on", cfg.Bind)

	srv := &http.Server{
		Addr:    cfg.Bind,
		Handler: r,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 30,
		IdleTimeout:  time.Second * 60,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// Accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)
	// Block until we receive our signal.
	<-c
	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	log.Info("Go-Healthz shutting down")
	os.Exit(0)
}

func (cfg *StatusConfig) Unique() {
	svcs := make(map[string]int)
	for i, svc := range cfg.Services {
		if previous, present := svcs[svc.Name]; present {
			copy(cfg.Services[previous:], cfg.Services[previous+1:])
			cfg.Services[len(cfg.Services)-1] = nil
			cfg.Services = cfg.Services[:len(cfg.Services)-1]
		}
		svcs[svc.Name] = i
	}

	cmds := make(map[string]int)
	for i, cmd := range cfg.Commands {
		if previous, present := cmds[cmd.Name]; present {
			copy(cfg.Commands[previous:], cfg.Commands[previous+1:])
			cfg.Commands[len(cfg.Commands)-1] = nil
			cfg.Commands = cfg.Commands[:len(cfg.Commands)-1]
		}
		cmds[cmd.Name] = i
	}

	reqs := make(map[string]int)
	for i, req := range cfg.Requests {
		if previous, present := reqs[req.Name]; present {
			copy(cfg.Requests[previous:], cfg.Requests[previous+1:])
			cfg.Requests[len(cfg.Requests)-1] = nil
			cfg.Requests = cfg.Requests[:len(cfg.Requests)-1]
		}
		reqs[req.Name] = i
	}
}
