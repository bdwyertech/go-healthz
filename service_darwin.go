// +build darwin

package main

import (
	log "github.com/sirupsen/logrus"
)

type SvcStatus struct {
	Name    string
	Healthy bool
	State   map[string]interface{}
}

func (svc *Service) Check() (status SvcStatus, err error) {
	status.Name = "Services not yet supported on OS X: " + svc.Name
	log.Println("Services not yet supported on OS X")
	return
}
