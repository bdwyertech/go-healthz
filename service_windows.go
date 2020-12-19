// +build windows

package main

import (
	"time"

	log "github.com/sirupsen/logrus"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type SvcStatus struct {
	Name      string
	Healthy   bool
	State     svc.Status
	Timestamp time.Time
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
		log.Warnf("Could not open service %v: %v", svc.Name, err)
		return
	}
	defer s.Close()

	status.State, err = s.Query()
	if err != nil {
		log.Warnf("Could not query status of service %v: %v", svc.Name, err)
		return
	}

	if status.State.State == windows.SERVICE_RUNNING {
		status.Healthy = true
	}

	status.Timestamp = time.Now()

	return
}
