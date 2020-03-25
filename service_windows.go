// +build windows

package main

import (
	"log"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type SvcStatus struct {
	Name    string
	Healthy bool
	State   svc.Status
}

func ServiceStatus(service string) (status SvcStatus, err error) {
	status.Name = service
	m, err := mgr.Connect()
	if err != nil {
		log.Fatal("SCM connection failed: ", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(service)
	if err != nil {
		log.Printf("Could not open service %v: %v", service, err)
		return
	}

	status.State, err = s.Query()
	if err != nil {
		log.Printf("Could not query service status for %v: %v", service, err)
		return
	}

	if status.State.State == windows.SERVICE_RUNNING {
		status.Healthy = true
	}

	return
}
