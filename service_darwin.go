//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type SvcStatus struct {
	Name      string
	Healthy   bool
	Reason    string                 `json:",omitempty"`
	State     map[string]interface{} `json:",omitempty"`
	Timestamp time.Time
}

func (svc *Service) Check() (status SvcStatus, err error) {
	status.Name = svc.Name
	out, err := exec.Command("/bin/launchctl", "list").Output()
	if err != nil {
		return
	}

	var pid, lastExitStatus string
	var found bool
	scanner := bufio.NewScanner(bytes.NewBuffer(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[2] == svc.Name {
			found = true
			if fields[0] != "-" {
				pid = fields[0]
			}
			if fields[1] != "-" {
				lastExitStatus = fields[1]
			}
		}
	}

	if found {
		// If pid is set and > 0, then clear lastExitStatus which is the
		// exit status of the previous run and doesn't mean anything for
		// the current state. Clearing it to avoid confusion.
		pidInt, _ := strconv.ParseInt(pid, 0, 64)
		if pid != "" && pidInt > 0 {
			lastExitStatus = ""
		}
		status.State = make(map[string]interface{})
		status.State["label"] = svc.Name
		status.State["pid"] = pid
		status.State["lastExitStatus"] = lastExitStatus
		if pid != "" {
			status.Healthy = true
		}
		return
	}

	err = errors.New("Service not found")

	status.Timestamp = time.Now()

	return
}
