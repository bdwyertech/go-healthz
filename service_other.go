// +build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/coreos/go-systemd/v22/dbus"
)

type SvcStatus struct {
	Name    string
	Healthy bool
	State   map[string]interface{}
}

func (svc *Service) Check() (status SvcStatus, err error) {
	switch {
	case isSystemD():
		return svc.checkSystemD()
	case isRedhatSysV(), isDebianSysV():
		return svc.checkSysV()
	default:
		log.Fatal("Service checks on this platform are not supported yet!")
	}

	return
}

func (svc *Service) checkSystemD() (status SvcStatus, err error) {
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

func (svc *Service) checkSysV() (status SvcStatus, err error) {
	status.Name = svc.Name

	status.State = make(map[string]interface{})
	cmd := exec.Command("service", svc.Name, "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		cmdString := strings.Join([]string{"service", svc.Name, "status"}, " ")
		err = fmt.Errorf("%q failed: %v, %s", cmdString, err, out)
	}

	output := strings.TrimSpace(string(out))
	status.State["Output"] = output

	if strings.Contains(output, "is running") {
		status.Healthy = true
	}

	return
}

func isSystemD() bool {
	// https://www.freedesktop.org/software/systemd/man/sd_booted.html
	if _, err := os.Stat("/run/systemd/system/"); err != nil {
		return false
	}
	return true
}

func isDebianSysV() bool {
	if _, err := os.Stat("/lib/lsb/init-functions"); err != nil {
		return false
	}
	if _, err := os.Stat("/sbin/start-stop-daemon"); err != nil {
		return false
	}
	return true
}

func isRedhatSysV() bool {
	if _, err := os.Stat("/etc/rc.d/init.d/functions"); err != nil {
		return false
	}
	return true
}
