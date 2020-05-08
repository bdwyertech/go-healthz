// +build windows

// Windows Service Handler

package main

import (
	"flag"

	log "github.com/sirupsen/logrus"

	"github.com/kardianos/service"
)

type program struct {
	configPath string
	exit       chan struct{}
}

var svcFlag string

func init() {
	if flag.Lookup("service") == nil {
		flag.StringVar(&svcFlag, "service", "", "Control the Windows System service.")
	}
}

func RunWindows(cfgPath string) {
	svcConfig := &service.Config{
		Name:        "Go-Healthz",
		DisplayName: "Go Healthz",
		Description: "Go Healthz Healthcheck Daemon",
	}

	prg := &program{}
	prg.configPath = cfgPath
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if len(svcFlag) != 0 {
		err := service.Control(s, svcFlag)
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
	<-p.exit
	return
}

func (p *program) Stop(s service.Service) (err error) {
	close(p.exit)
	return
}
