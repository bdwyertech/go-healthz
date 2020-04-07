// Encoding: UTF-8

package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kofalt/go-memoize"
)

type CmdStatus struct {
	Name    string
	Command string
	Healthy bool
	Output  string
	Error   string
	Code    int
}

type Command struct {
	Name      string `yaml:"name"`
	Command   string `yaml:"cmd"`
	Frequency string `yaml:"frequency"`
	Timeout   string `yaml:"timeout"`
	Sensitive bool   `yaml:"sensitive"`
	cache     *memoize.Memoizer
}

func (cmd *Command) Cache() (cache *memoize.Memoizer) {
	if cmd.cache != nil {
		return cmd.cache
	}

	duration := 5 * time.Second
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
	status.Command = command.Command

	timeout := 5 * time.Second
	if command.Timeout != "" {
		var err error
		if timeout, err = time.ParseDuration(command.Timeout); err != nil {
			log.Fatal(err)
		}
		if timeout > 20*time.Second {
			log.Warn(command.Name, ": Command timeout cannot be longer than 20 seconds!")
			timeout = 20 * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmdArgs := strings.Fields(command.Command)

	cmd := exec.CommandContext(ctx, cmdArgs[0])
	if len(cmdArgs) > 1 {
		cmd.Args = cmdArgs[1:]
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	log.Debugln("Executing Command:", command.Command)
	if err = cmd.Run(); err != nil {
		status.Healthy = false
		if ctxErr := ctx.Err(); ctxErr == context.DeadlineExceeded {
			status.Error = "Command timed out"
			log.Warnf("%v: %v", command.Name, status.Error)
		} else {
			status.Error = err.Error()
		}
		log.Warnf("%v: %v", command.Name, status.Error)

		if !command.Sensitive {
			if errString := strings.TrimSpace(string(stderr.Bytes())); errString != "" {
				status.Output = errString
				log.Warnf("%v: %v", command.Name, errString)
			}
			status.Code = cmd.ProcessState.ExitCode()
		}
		return
	}
	status.Code = cmd.ProcessState.ExitCode()
	status.Healthy = true
	if command.Sensitive {
		status.Command = "SENSITIVE: REDACTED"
		status.Output = "SENSITIVE: REDACTED"
		status.Error = "SENSITIVE: REDACTED"
		return
	}
	status.Output = strings.TrimSpace(string(stdout.Bytes()))
	status.Error = strings.TrimSpace(string(stderr.Bytes()))
	return
}
