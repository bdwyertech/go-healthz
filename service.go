// Encoding: UTF-8

package main

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kofalt/go-memoize"
)

type Service struct {
	Name  string `yaml:"name"`
	Cache string `yaml:"cache"`
	cache *memoize.Memoizer
}

func (svc *Service) Cached() (cache *memoize.Memoizer) {
	if svc.cache != nil {
		return svc.cache
	}

	duration := 5 * time.Second
	if svc.Cache != "" {
		var err error
		if duration, err = time.ParseDuration(svc.Cache); err != nil {
			log.Fatal(err)
		}
	}

	svc.cache = memoize.NewMemoizer(duration, 5*time.Minute)

	return svc.cache
}

func (svc *Service) Status() (status SvcStatus, err error) {
	s, err, _ := svc.Cached().Memoize(svc.Name, func() (interface{}, error) {
		return svc.Check()
	})

	status, ok := s.(SvcStatus)
	if !ok {
		log.Fatal("Unable to convert response into SvcStatus")
	}

	if !status.Healthy {
		// Check if disabled remotely via SRV Record
		if dnsRecord, disabled := RemotelyDisabled(svc.Name); disabled {
			status.Reason = "disabled remotely via " + dnsRecord
			log.Infoln("Service", svc.Name, status.Reason)
			status.Healthy = true
		}
	}

	return
}
