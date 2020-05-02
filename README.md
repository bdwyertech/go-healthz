# Go-Healthz

:wrench:  Simple extensible healthz service

[![GoDoc](https://godoc.org/github.com/bdwyertech/go-healthz?status.svg)](https://godoc.org/github.com/bdwyertech/go-healthz)
[![Build Status](https://github.com/bdwyertech/go-healthz/workflows/Go/badge.svg?branch=master)](https://github.com/bdwyertech/go-healthz/actions?query=workflow%3AGo+branch%3Amaster)
[![Coverage Status](https://coveralls.io/repos/bdwyertech/go-healthz/badge.svg?branch=dev&service=github)](https://coveralls.io/github/bdwyertech/go-healthz?branch=master)
[![Gitter](https://img.shields.io/badge/Gitter-bdwyertech%2Fgo--healthz-brightgreen.svg)][gitter]

[gitter]: https://gitter.im/bdwyertech/go-healthz

## Overview

This is a simple web service designed to enable simple health checks for services which otherwise do not expose their own.

Health checks are performed in real-time, per request.  For this reason, a flexible caching mechanism has been implemented with sane defaults to prevent "expensive" checks from degrading performance.  Even so, on AWS, you'd want to limit inbound traffic from the Load Balancer only as they are the source of health check traffic.

Both caching and timeouts are configurable.  They both default to 5s, with timeouts limited to a maximum of 20s.  Both accept a [Go Duration String](https://golang.org/pkg/time/#ParseDuration).

### Global Route
The global `/` route returns a 200 if all commands & services are healthy, and a 503 Service Unavailable if unhealthy.

### Command-Specific Route
Command-specific routes are available and correspond to the name of the command.  For example, `/command/MyCommandName`

### Request-Specific Route
Request-specific routes are available and correspond to the name of the request.  For example, `/request/MyRequestName`

### Service-Specific Route
Service-specific routes are available and correspond to the name of the service.  For example, `/service/MyAppServiceName`

### Passthrough Proxies
This service supports pass-through to localhost services.  You must explicitly declare supported verbs.

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
  # Darwin
  - name: com.apple.SoftwareUpdateNotificationManager
  - name: com.apple.Spotlight

commands:
  - name: 'Who am I?'
    cmd: whoami
  - name: 'Secret'
    cmd: 'whoami'
    sensitive: true
  - name: 'Date'
    cmd: 'date'
    timeout: 1s
    cache: 5s
  - name: 'PowerShell'
    cmd: 'powershell.exe -NonInteractive -Command Get-Service WManSvc | select DisplayName, Status | Format-Table -HideTableHeaders'

proxies:
  - name: nginx
    port: 8080
    methods:
      - GET
      - HEAD

requests:
  - name: Get
    url: https://postman-echo.com/get?foo1=bar1&foo2=bar2
    method: GET
    timeout: 2s
    insecure: true
    codes:
      - 200
  - name: Post
    cache: 30s
    url: https://postman-echo.com/post
    method: POST
    body: foo=bar
    headers:
      Content-Type: application/x-www-form-urlencoded
    codes:
      - 200
  - name: PostJSON
    cache: 30s
    url: https://postman-echo.com/post
    method: POST
    body: >
     {
      "test": true,
      "app": "go-healthz",
      "#": 11
     }
    headers:
      Content-Type: application/json
    codes:
      - 200
```

#### SystemD Unit
```ini
[Unit]
Description=Go-Healthz Healthcheck Daemon
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/go-healthz -config /etc/go-healthz.yml
OOMScoreAdjust=-500
Restart=always

[Install]
WantedBy=multi-user.target
```
