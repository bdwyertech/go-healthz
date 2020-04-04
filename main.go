// Encoding: UTF-8

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v2"
)

type StatusConfig struct {
	Bind     string     `yaml:"bind"`
	Commands []*Command `yaml:"commands"`
	Services []*Service `yaml:"services"`
}

type GlobalStatus struct {
	Healthy  bool
	Services []SvcStatus `json:",omitempty"`
	Commands []CmdStatus `json:",omitempty"`
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

	mux := http.NewServeMux()

	for _, _svc := range cfg.Services {
		svc := _svc
		mux.HandleFunc("/service/"+svc.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := svc.Status()
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		})
	}

	for _, _cmd := range cfg.Commands {
		cmd := _cmd
		mux.HandleFunc("/command/"+cmd.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := cmd.Status()
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		})
	}

	// Ignore Favicon Requests (Browser)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) {})

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
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

		if global.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(global)
	})

	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0:8080"
	}
	log.Infoln("Go-Healthz listening on", cfg.Bind)

	srv := &http.Server{
		Addr:    cfg.Bind,
		Handler: mux,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	log.Fatal(srv.ListenAndServe())
}
