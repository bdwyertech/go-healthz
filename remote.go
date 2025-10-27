// Encoding: UTF-8

package main

import (
	"context"
	"net"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/jellydator/ttlcache/v3"
)

var remotelyDisabled = ttlcache.New[string, string](
	ttlcache.WithTTL[string, string](5*time.Minute),
	ttlcache.WithDisableTouchOnHit[string, string](),
)

func Remote(dnsRecords []string) {
	if len(dnsRecords) > 0 {
		for _, r := range dnsRecords {
			go RemoteFetcher(r)
		}
	}
}

func RemoteFetcher(dnsRecord string) {
	for {
		timeout := 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		go func() {
			defer cancel()
			txtrecords, err := net.DefaultResolver.LookupTXT(ctx, dnsRecord)
			if err != nil {
				if ctx.Err() == nil {
					if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
						log.Debug(dnsErr)
						return
					}
					log.Error(err)
				}
				return
			}
			for _, txt := range txtrecords {
				for _, entry := range strings.Split(txt, ",") {
					entry := strings.SplitN(entry, "=", 2)
					if len(entry) != 2 {
						log.Debugln("Invalid DNS entry:", dnsRecord, entry)
						continue
					}
					if strings.EqualFold(strings.TrimSpace(entry[1]), "disabled") {
						remotelyDisabled.Set(strings.TrimSpace(entry[0]), dnsRecord, ttlcache.DefaultTTL)
					}
				}
			}
		}()
		<-ctx.Done()
		switch ctxErr := ctx.Err(); ctxErr {
		case context.Canceled:
			// Do Nothing
		case context.DeadlineExceeded:
			log.Errorln("DNS lookup timed out:", dnsRecord)
		default:
			log.Errorln(ctxErr, dnsRecord)
		}
		time.Sleep(2 * time.Minute)
	}
}

func RemotelyDisabled(check string) (dnsRecord string, disabled bool) {
	// Check if disabled remotely via SRV Record
	if remotelyDisabled.Has(check) {
		return remotelyDisabled.Get(check).Value(), true
	}
	return
}
