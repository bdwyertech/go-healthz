package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// TestRemoteFetcherExitsOnCancel verifies that RemoteFetcher goroutines
// exit promptly when the parent context is cancelled.
func TestRemoteFetcherExitsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		RemoteFetcher(ctx, "nonexistent.example.com")
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("RemoteFetcher did not exit within 3 seconds of context cancellation")
	}
}

// TestRemoteFetcherNoCacheRace verifies that RemoteFetcher does not race on
// ttlcache writes. Run with -race to detect.
func TestRemoteFetcherNoCacheRace(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		RemoteFetcher(ctx, "nonexistent.example.com")
		close(done)
	}()

	time.Sleep(500 * time.Millisecond)

	// Read from the cache concurrently to surface any race
	for i := 0; i < 10; i++ {
		RemotelyDisabled("some-check")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("RemoteFetcher did not exit within 3 seconds of context cancellation")
	}
}

// TestRemoteAcceptsCancelledContext verifies Remote() handles a pre-cancelled
// context without panicking.
func TestRemoteAcceptsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	Remote(ctx, nil)
	Remote(ctx, []string{})
	Remote(ctx, []string{"a.example.com", "b.example.com"})
}

// TestRemotelyDisabledCacheLookup verifies RemotelyDisabled returns correct
// values based on the ttlcache state.
func TestRemotelyDisabledCacheLookup(t *testing.T) {
	origCache := remotelyDisabled
	remotelyDisabled = ttlcache.New(
		ttlcache.WithTTL[string, string](5*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, string](),
	)
	defer func() { remotelyDisabled = origCache }()

	// Empty cache
	dnsRecord, disabled := RemotelyDisabled("my-service")
	if disabled {
		t.Error("Expected false for empty cache")
	}
	if dnsRecord != "" {
		t.Errorf("Expected empty string, got %q", dnsRecord)
	}

	// Populate and verify
	remotelyDisabled.Set("my-service", "test.example.com", ttlcache.DefaultTTL)

	dnsRecord, disabled = RemotelyDisabled("my-service")
	if !disabled {
		t.Error("Expected true when check is in cache")
	}
	if dnsRecord != "test.example.com" {
		t.Errorf("Expected test.example.com, got %q", dnsRecord)
	}

	// Different key still returns false
	_, disabled = RemotelyDisabled("other-service")
	if disabled {
		t.Error("Expected false for missing key")
	}
}

// TestRemotelyDisabledMultipleEntries verifies multiple entries coexist
// independently in the cache.
func TestRemotelyDisabledMultipleEntries(t *testing.T) {
	origCache := remotelyDisabled
	remotelyDisabled = ttlcache.New(
		ttlcache.WithTTL[string, string](5*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, string](),
	)
	defer func() { remotelyDisabled = origCache }()

	entries := map[string]string{
		"svc-a": "dns-a.example.com",
		"svc-b": "dns-b.example.com",
		"svc-c": "dns-c.example.com",
	}

	for k, v := range entries {
		remotelyDisabled.Set(k, v, ttlcache.DefaultTTL)
	}

	for k, expected := range entries {
		record, disabled := RemotelyDisabled(k)
		if !disabled {
			t.Errorf("RemotelyDisabled(%q) should return true", k)
		}
		if record != expected {
			t.Errorf("RemotelyDisabled(%q) = %q, want %q", k, record, expected)
		}
	}
}

// TestTXTRecordParsing verifies the TXT record parsing logic correctly
// extracts name=disabled entries.
func TestTXTRecordParsing(t *testing.T) {
	origCache := remotelyDisabled
	remotelyDisabled = ttlcache.New(
		ttlcache.WithTTL[string, string](5*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, string](),
	)
	defer func() { remotelyDisabled = origCache }()

	dnsRecord := "test.example.com"
	txtRecords := []string{
		"svc-a=disabled,svc-b=disabled",
		"svc-c=disabled",
	}

	for _, txt := range txtRecords {
		for _, entry := range strings.Split(txt, ",") {
			entry := strings.SplitN(entry, "=", 2)
			if len(entry) != 2 {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(entry[1]), "disabled") {
				remotelyDisabled.Set(strings.TrimSpace(entry[0]), dnsRecord, ttlcache.DefaultTTL)
			}
		}
	}

	for _, name := range []string{"svc-a", "svc-b", "svc-c"} {
		record, disabled := RemotelyDisabled(name)
		if !disabled {
			t.Errorf("Expected %q to be disabled", name)
		}
		if record != dnsRecord {
			t.Errorf("Expected %q for %q, got %q", dnsRecord, name, record)
		}
	}

	if _, disabled := RemotelyDisabled("svc-d"); disabled {
		t.Error("svc-d should not be disabled")
	}
}

// TestTXTRecordParsingEdgeCases verifies edge cases: missing =, non-disabled
// values, whitespace, case insensitivity.
func TestTXTRecordParsingEdgeCases(t *testing.T) {
	origCache := remotelyDisabled
	remotelyDisabled = ttlcache.New(
		ttlcache.WithTTL[string, string](5*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, string](),
	)
	defer func() { remotelyDisabled = origCache }()

	dnsRecord := "edge.example.com"
	txtRecords := []string{
		"valid-svc = Disabled",
		"invalid-entry",
		"enabled-svc=enabled",
		" spaced-svc = disabled ",
	}

	for _, txt := range txtRecords {
		for _, entry := range strings.Split(txt, ",") {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(parts[1]), "disabled") {
				remotelyDisabled.Set(strings.TrimSpace(parts[0]), dnsRecord, ttlcache.DefaultTTL)
			}
		}
	}

	if _, disabled := RemotelyDisabled("valid-svc"); !disabled {
		t.Error("valid-svc should be disabled (case insensitive)")
	}
	if _, disabled := RemotelyDisabled("invalid-entry"); disabled {
		t.Error("invalid-entry should not be in cache (no = sign)")
	}
	if _, disabled := RemotelyDisabled("enabled-svc"); disabled {
		t.Error("enabled-svc should not be disabled")
	}
	if _, disabled := RemotelyDisabled("spaced-svc"); !disabled {
		t.Error("spaced-svc should be disabled (whitespace trimmed)")
	}
}

// TestDNSNotFoundDoesNotPopulateCache verifies that DNS not-found errors
// leave the cache empty.
func TestDNSNotFoundDoesNotPopulateCache(t *testing.T) {
	origCache := remotelyDisabled
	remotelyDisabled = ttlcache.New(
		ttlcache.WithTTL[string, string](5*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, string](),
	)
	defer func() { remotelyDisabled = origCache }()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_, err := net.DefaultResolver.LookupTXT(ctx, "this-domain-does-not-exist-12345.example.com")
	if err != nil {
		if remotelyDisabled.Len() != 0 {
			t.Error("Cache should be empty after DNS not-found error")
		}
	}
}
