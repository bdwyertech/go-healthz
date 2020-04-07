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
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
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

func init() {
	log.SetFormatter(&log.TextFormatter{
		ForceColors: true,
	})
	log.SetLevel(log.DebugLevel)
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	dcfg := filepath.Join(dir, "go-healthz.yml")
	cfgPath := flag.String("config", dcfg, "Path to configuration file")
	flag.Parse()

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
		for _, svc := range cfg.Services {
			status, _ := svc.Status()
			if !status.Healthy {
				global.Healthy = false
			}
			global.Services = append(global.Services, status)
		}

		for _, cmd := range cfg.Commands {
			status, _ := cmd.Status()
			if !status.Healthy {
				global.Healthy = false
			}
			global.Commands = append(global.Commands, status)
		}

		for _, req := range cfg.Requests {
			status, _ := req.Status()
			if !status.Healthy {
				global.Healthy = false
			}
			global.Requests = append(global.Requests, status)
		}

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
