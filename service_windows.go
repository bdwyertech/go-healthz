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

func (svc *Service) Check() (status SvcStatus, err error) {
	status.Name = svc.Name
	m, err := mgr.Connect()
	if err != nil {
		log.Fatal("SCM connection failed: ", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(svc.Name)
	if err != nil {
		log.Printf("Could not open service %v: %v", svc.Name, err)
		return
	}

	status.State, err = s.Query()
	if err != nil {
		log.Printf("Could not query status of service %v: %v", svc.Name, err)
		return
	}

	if status.State.State == windows.SERVICE_RUNNING {
		status.Healthy = true
	}

	return
}
