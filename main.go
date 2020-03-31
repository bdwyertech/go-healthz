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

	"github.com/kardianos/service"
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
	svcFlag := flag.String("service", "", "Control the system service.")
	flag.Parse()

	svcConfig := &service.Config{
		Name:        "Go-Healthz",
		DisplayName: "Go Healthz",
		Description: "Go Healthz Healthcheck Daemon",
		// Dependencies: []string{
		// 	"Requires=network.target",
		// 	"After=network-online.target syslog.target"},
		// Option: options,
	}
	if runtime.GOOS != "windows" {
		// options := make(service.KeyValue)
		// options["Restart"] = "on-success"
		// options["SuccessExitStatus"] = "1 2 8 SIGKILL"
		svcConfig.Option = make(service.KeyValue)
		svcConfig.Option["Restart"] = "always"
		svcConfig.Option["SuccessExitStatus"] = "1 2 8 SIGKILL"
		svcConfig.Dependencies = []string{
			"Requires=network.target",
			"After=network-online.target syslog.target",
		}
	}

	prg := &program{}
	prg.configPath = *cfgPath
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	if err = s.Run(); err != nil {
		log.Fatal(err)
	}
}

type program struct {
	configPath string
	exit       chan struct{}
}

func (p *program) Start(s service.Service) (err error) {
	if service.Interactive() {
		log.Println("Running in terminal.")
	} else {
		log.Println("Running under service manager.")
	}
	p.exit = make(chan struct{})

	go p.run()

	return
}

func (p *program) run() (err error) {
	Run(p.configPath)
	for {
		select {
		case <-p.exit:
			return
		}
	}
}

func (p *program) Stop(s service.Service) (err error) {
	close(p.exit)
	return
}
