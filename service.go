// Encoding: UTF-8

package main

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kofalt/go-memoize"
)

type Service struct {
	Name      string `yaml:"name"`
	Frequency string `yaml:"frequency"`
	cache     *memoize.Memoizer
}

func (svc *Service) Cache() (cache *memoize.Memoizer) {
	if svc.cache != nil {
		return svc.cache
	}

	duration := 5 * time.Second
	if svc.Frequency != "" {
		var err error
		if duration, err = time.ParseDuration(svc.Frequency); err != nil {
			log.Fatal(err)
		}
	}

	svc.cache = memoize.NewMemoizer(duration, 5*time.Minute)

	return svc.cache
}

func (svc *Service) Status() (status SvcStatus, err error) {
	s, err, _ := svc.Cache().Memoize(svc.Name, func() (interface{}, error) {
		return svc.Check()
	})

	status, ok := s.(SvcStatus)
	if !ok {
		log.Fatal("Unable to convert response into SvcStatus")
	}
	return
}
