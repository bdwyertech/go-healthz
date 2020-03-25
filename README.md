# Go-Healthz

:wrench:  Simple extensible healthz service

[![GoDoc](https://godoc.org/github.com/bdwyertech/go-healthz?status.svg)](https://godoc.org/github.com/bdwyertech/go-healthz)
[![Build Status](https://github.com/bdwyertech/go-healthz/workflows/Go/badge.svg?branch=master)](https://github.com/bdwyertech/go-healthz/actions?query=workflow%3AGo+branch%3Amaster)
[![Coverage Status](https://coveralls.io/repos/bdwyertech/go-healthz/badge.svg?branch=dev&service=github)](https://coveralls.io/github/bdwyertech/go-healthz?branch=master)

## Overview

This is a simple web service designed to enable simple health checks for services which otherwise do not expose their own.

Go-Healthz performs health checks are performed in real-time, per request.  For this reason, you should use firewall rules to prevent DDoS'ing this endpoint.  For example, on AWS, you'd limit inbound traffic from the Load Balancer only because the Load Balancers are the source of health check traffic.

### Global Route
The global `/healthz` route returns a 200 if all services are healthy, and a 503 Service Unavailable if unhealthy.

### Service-Specific Route
Service-specific routes are available underneath `/healthz/` and correspond to the name of the service.  For example, `/healthz/MyAppServiceName`

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
```
