# Go-Healthz

:wrench:  Simple extensible healthz service

[![GoDoc](https://godoc.org/github.com/bdwyertech/go-healthz?status.svg)](https://godoc.org/github.com/bdwyertech/go-healthz)
[![Build Status](https://github.com/bdwyertech/go-healthz/workflows/Go/badge.svg?branch=master)](https://github.com/bdwyertech/go-healthz/actions?query=workflow%3AGo+branch%3Amaster)
[![Coverage Status](https://coveralls.io/repos/bdwyertech/go-healthz/badge.svg?branch=dev&service=github)](https://coveralls.io/github/bdwyertech/go-healthz?branch=master)

## Overview

This is a simple web service designed to enable simple health checks for services which otherwise do not expose their own.

Go-Healthz performs health checks are performed in real-time, per request.  For this reason, you should use firewall rules to prevent DDoS'ing this endpoint.  For example, on AWS, you'd limit inbound traffic from the Load Balancer only because the Load Balancers are the source of health check traffic.

Frequency can be configured which will cache the results for the specified period of time.  This value is a [Go Duration String](https://golang.org/pkg/time/#ParseDuration)

### Global Route
The global `/` route returns a 200 if all commands & services are healthy, and a 503 Service Unavailable if unhealthy.

### Command-Specific Route
Command-specific routes are available and correspond to the name of the command.  For example, `/command/MyCommandName`

### Service-Specific Route
Service-specific routes are available and correspond to the name of the service.  For example, `/service/MyAppServiceName`

### Example Config
```yaml
bind: 0.0.0.0:3000

services:
  # Windows
  - name: MyAppService
  - name: MyDatabaseService
  # Linux (SystemD)
  - name: my-application.service
  - name: mysqld.service

commands:
  - name: 'Who am I?'
    cmd: whoami
  - name: 'Secret'
    cmd: 'whoami'
    sensitive: true
  - name: 'Date'
    cmd: 'date'
    frequency: 5s
  - name: 'PowerShell'
    cmd: 'powershell.exe -NonInteractive -Command Get-Service WManSvc | select DisplayName, Status | Format-Table -HideTableHeaders'
```

#### SystemD Unit
```sh
[Unit]
Description=Go-Healthz Healthcheck Daemon

[Service]
ExecStart=/usr/local/bin/go-healthz -config /etc/go-healthz.yml

[Install]
WantedBy=multi-user.target
```
