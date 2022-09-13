// Encoding: UTF-8

package main

import (
	"context"
	"encoding/json"
	"flag"
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
	"gopkg.in/yaml.v3"
)

type StatusConfig struct {
	Bind     string     `yaml:"bind"`
	Remotes  []string   `yaml:"remotes"`
	Commands []*Command `yaml:"commands"`
	Services []*Service `yaml:"services"`
	Proxies  []*Proxy   `yaml:"proxies"`
	Requests []*Request `yaml:"requests"`
}

type GlobalStatus struct {
	Healthy        bool
	UnhealthyCount int
	Reason         string          `json:",omitempty"`
	Services       []SvcStatus     `json:",omitempty"`
	Commands       []CmdStatus     `json:",omitempty"`
	Requests       []RequestStatus `json:",omitempty"`
}

func init() {
	if os.Getenv("HEALTHZ_DEBUG") != "" {
		log.SetLevel(log.DebugLevel)
	}
	log.SetFormatter(&log.JSONFormatter{})
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
	cfgFile, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	var cfg StatusConfig
	if err = yaml.Unmarshal(cfgFile, &cfg); err != nil {
		log.Fatal(err)
	}

	// Background Remote Disable
	Remote(cfg.Remotes)

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

	// Global Force Unhealthy semaphore
	globalSemaphore, err := filepath.Abs(cfgPath + ".unhealthy")
	if err != nil {
		log.Fatal(err)
	}

	r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true

		if _, err := os.Stat(globalSemaphore); err == nil {
			global.Healthy = false
			global.Reason = "Global unhealthy semaphore exists: " + globalSemaphore
			log.Warnln(global.Reason)
		}

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
					log.Warnln("Service unhealthy:", svc.Name)
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
					log.Warnln("Command unhealthy:", cmd.Name)
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
					log.Warnln("Request unhealthy:", req.Name)
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
