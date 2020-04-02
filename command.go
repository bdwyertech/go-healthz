// Encoding: UTF-8

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kofalt/go-memoize"
)

type CmdStatus struct {
	Name    string
	Healthy bool
	Output  string
	Error   string
	Code    int
}

type Command struct {
	Name      string `yaml:"name"`
	Command   string `yaml:"cmd"`
	Frequency string `yaml:"frequency"`
	Sensitive bool   `yaml:"sensitive"`
	cache     *memoize.Memoizer
}

func (cmd *Command) Cache() (cache *memoize.Memoizer) {
	if cmd.cache != nil {
		return cmd.cache
	}

	duration := 30 * time.Second
	if cmd.Frequency != "" {
		var err error
		if duration, err = time.ParseDuration(cmd.Frequency); err != nil {
			log.Fatal(err)
		}
	}

	cmd.cache = memoize.NewMemoizer(duration, 5*time.Minute)

	return cmd.cache
}

func (cmd *Command) Status() (status CmdStatus, err error) {
	s, err, _ := cmd.Cache().Memoize(cmd.Name, func() (interface{}, error) {
		return cmd.Run()
	})

	status, ok := s.(CmdStatus)
	if !ok {
		log.Fatal("Unable to convert response into CmdStatus")
	}
	return
}

func (command *Command) Run() (status CmdStatus, err error) {
	status.Name = command.Name
	cmdArgs := strings.Fields(command.Command)
	cmd := exec.Command(cmdArgs[0])
	if len(cmdArgs) > 1 {
		cmd.Args = cmdArgs[1:]
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		log.Println(err)
		status.Healthy = false
		status.Error = err.Error()
		if !command.Sensitive {
			status.Code = cmd.ProcessState.ExitCode()
			// out, err = cmd.Output
			// if err != nil {
			// 	log.Println(err)
			// 	return
			// }
			// status.Output = string(out)
		}
		return
	}
	status.Code = cmd.ProcessState.ExitCode()
	status.Healthy = true
	if command.Sensitive {
		status.Output = "SENSITIVE: REDACTED"
		status.Error = "SENSITIVE: REDACTED"
		return
	}
	status.Output = strings.TrimSpace(string(stdout.Bytes()))
	status.Error = strings.TrimSpace(string(stderr.Bytes()))
	return
}
