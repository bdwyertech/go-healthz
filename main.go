// Encoding: UTF-8

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	// "github.com/kofalt/go-memoize"
	"gopkg.in/yaml.v2"
)

type StatusConfig struct {
	Bind     string    `yaml:"bind"`
	Commands []Command `yaml:"commands"`
	Services []Service `yaml:"services"`
}

type GlobalStatus struct {
	Healthy  bool
	Services []SvcStatus
	Commands []CmdStatus
}

func Run(cfgPath string) {
	cfgFile, err := os.Open(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	defer cfgFile.Close()

	cfgBytes, err := ioutil.ReadAll(cfgFile)
	if err != nil {
		log.Fatal(err)
	}

	var cfg StatusConfig
	if err = yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	for _, _svc := range cfg.Services {
		svc := _svc
		mux.HandleFunc("/service/"+svc.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := ServiceStatus(svc.Name)
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
			status, _ := CommandStatus(cmd)
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		})
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true
		for _, svc := range cfg.Services {
			status, _ := ServiceStatus(svc.Name)
			if !status.Healthy {
				global.Healthy = false
			}
			global.Services = append(global.Services, status)
		}

		for _, cmd := range cfg.Commands {
			status, _ := CommandStatus(cmd)
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
	log.Println("Go-Healthz listening on", cfg.Bind)

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
