// Encoding: UTF-8

package main

import (
	"context"
	"net"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/ReneKroon/ttlcache/v2"
)

var remotelyDisabled ttlcache.SimpleCache = ttlcache.NewCache()

func Remote(dnsRecords []string) {
	if len(dnsRecords) > 0 {
		remotelyDisabled.SetTTL(time.Duration(5 * time.Minute))
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
			txtrecords, err := net.LookupTXT(dnsRecord)
			if err != nil {
				log.Error(err)
				return
			}
			for _, txt := range txtrecords {
				for _, entry := range strings.Split(txt, ",") {
					entry := strings.SplitN(entry, "=", 2)
					if len(entry) != 2 {
						log.Debugln("Invalid DNS entry:", dnsRecord, entry)
						continue
					}
					if strings.TrimSpace(entry[1]) == "disabled" {
						if err = remotelyDisabled.Set(strings.TrimSpace(entry[0]), dnsRecord); err != nil {
							log.Error(err)
						}
					}
				}
			}
		}()
		<-ctx.Done()
		switch ctxErr := ctx.Err(); ctxErr {
		case context.Canceled:
			// Do Nothing
		case context.DeadlineExceeded:
			log.Errorln("DNS Request timed out:", ctxErr)
		default:
			log.Error(ctxErr)
		}
		time.Sleep(2 * time.Minute)
	}
}

func RemotelyDisabled(check string) (dnsRecord string, disabled bool) {
	// Check if disabled remotely via SRV Record
	if v, err := remotelyDisabled.Get(check); err == nil {
		dnsRecord, disabled = v.(string)
	}
	return
}
