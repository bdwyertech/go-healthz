//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/coreos/go-systemd/v22/dbus"
)

type SvcStatus struct {
	Name      string
	Healthy   bool
	Reason    string                 `json:",omitempty"`
	State     map[string]interface{} `json:",omitempty"`
	Timestamp time.Time
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

	m, err := dbus.NewWithContext(context.Background())

	if err != nil {
		log.Fatal("SCM connection failed: ", err)
	}
	defer m.Close()

	s, err := m.GetAllPropertiesContext(context.Background(), svc.Name)
	if err != nil {
		log.Warnf("Could not open service %v: %v", svc.Name, err)
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

	status.Timestamp = time.Now()

	return
}

func (svc *Service) checkSysV() (status SvcStatus, err error) {
	status.Name = svc.Name

	status.State = make(map[string]interface{})

	timeout := 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "service", svc.Name, "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr == context.DeadlineExceeded {
			status.State["Error"] = fmt.Sprintf("Command timed out: %v", cmd.String())
		} else {
			err = fmt.Errorf("%v failed: %v, %v", cmd.String(), err, strings.TrimSpace(string(out)))
			status.State["Error"] = err.Error()
			return
		}
	}

	output := strings.TrimSpace(string(out))
	status.State["Output"] = output

	if strings.Contains(output, "is running") {
		status.Healthy = true
	}

	status.Timestamp = time.Now()

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
