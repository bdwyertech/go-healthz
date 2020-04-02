// +build !windows

package main

import (
	log "github.com/sirupsen/logrus"

	"github.com/coreos/go-systemd/v22/dbus"
)

type SvcStatus struct {
	Name    string
	Healthy bool
	State   map[string]interface{}
}

func (svc *Service) Check() (status SvcStatus, err error) {
	status.Name = svc.Name

	m, err := dbus.New()

	if err != nil {
		log.Fatal("SCM connection failed: ", err)
	}
	defer m.Close()

	s, err := m.GetAllProperties(svc.Name)
	if err != nil {
		log.Printf("Could not open service %v: %v", svc.Name, err)
		return
	}

	status.Healthy = true
	status.State = make(map[string]interface{})
	for _, v := range []string{"SubState", "StatusErrno", "StatusText"} {
		if v == "SubState" && s[v] != "running" {
			status.Healthy = false
		}
		status.State[v] = s[v]
	}

	return
}
