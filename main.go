// Encoding: UTF-8

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"

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

func main() {
	cfgPath := flag.String("config", "config.yml", "Path to configuration file")
	flag.Parse()
	cfgFile, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	defer cfgFile.Close()

	cfgBytes, _ := ioutil.ReadAll(cfgFile)

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
		cfg.Bind = "0.0.0.0:3000"
	}
	log.Println("Go-Healthz listening on", cfg.Bind)
	log.Fatal(http.ListenAndServe(cfg.Bind, mux))
}
