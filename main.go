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

	// "github.com/kofalt/go-memoize"
	"gopkg.in/yaml.v2"
)

type StatusConfig struct {
	Bind     string `yaml:"bind"`
	Services []struct {
		Name      string `yaml:"name"`
		Frequency string `yaml:"frequency"`
	} `yaml:"services"`
}

type GlobalStatus struct {
	Healthy  bool
	Services []SvcStatus
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

	allServices := []string{}

	for _, svc := range cfg.Services {
		mux.HandleFunc("/healthz/"+svc.Name, func(w http.ResponseWriter, _ *http.Request) {
			status, _ := ServiceStatus(svc.Name)
			w.Header().Set("Content-Type", "application/json")
			if status.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			json.NewEncoder(w).Encode(status)
		})

		allServices = append(allServices, svc.Name)
	}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true
		for _, svc := range allServices {
			status, _ := ServiceStatus(svc)
			if !status.Healthy {
				global.Healthy = false
			}
			global.Services = append(global.Services, status)
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
	log.Fatal(http.ListenAndServe(cfg.Bind, mux))
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
