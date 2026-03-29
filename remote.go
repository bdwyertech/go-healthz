// Encoding: UTF-8

package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/jellydator/ttlcache/v3"
)

var remotelyDisabled = ttlcache.New(
	ttlcache.WithTTL[string, string](5*time.Minute),
	ttlcache.WithDisableTouchOnHit[string, string](),
)

func Remote(ctx context.Context, dnsRecords []string) {
	for _, r := range dnsRecords {
		go RemoteFetcher(ctx, r)
	}
}

func RemoteFetcher(ctx context.Context, dnsRecord string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		txtrecords, err := net.DefaultResolver.LookupTXT(lookupCtx, dnsRecord)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return // parent context cancelled, exit
			}
			if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
				log.Debug(dnsErr)
			} else if !errors.Is(err, context.DeadlineExceeded) {
				log.Error(err)
			} else {
				log.Errorln("DNS lookup timed out:", dnsRecord)
			}
		} else {
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
		}

		select {
		case <-time.After(2 * time.Minute):
		case <-ctx.Done():
			return
		}
	}
}

func RemotelyDisabled(check string) (dnsRecord string, disabled bool) {
	// Check if disabled remotely via SRV Record
	if remotelyDisabled.Has(check) {
		return remotelyDisabled.Get(check).Value(), true
	}
	return
}
