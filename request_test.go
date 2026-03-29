package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSensitiveRequestClosesBody verifies that sensitive requests properly
// close and drain the response body, allowing TCP connection reuse.
func TestSensitiveRequestClosesBody(t *testing.T) {
	var newConnCount atomic.Int32

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"secret": "value"}`))
	}))
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnCount.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	req := &Request{
		Name:      "sensitive-body-test",
		Method:    "GET",
		Url:       server.URL,
		Codes:     []int{200},
		Sensitive: true,
	}

	// First call establishes a TCP connection
	status1, err := req.Run()
	assert.NoError(t, err)
	assert.Equal(t, 200, status1.StatusCode)

	time.Sleep(100 * time.Millisecond)
	connsBefore := newConnCount.Load()

	// Second call should reuse the connection if the body was properly closed
	status2, err := req.Run()
	assert.NoError(t, err)
	assert.Equal(t, 200, status2.StatusCode)

	time.Sleep(100 * time.Millisecond)
	connsAfter := newConnCount.Load()

	assert.Equal(t, connsBefore, connsAfter,
		"Second request opened a new connection (%d -> %d); body was not closed/drained",
		connsBefore, connsAfter)
}

// TestTransportReuse verifies that sequential Run() calls on the same Request
// reuse a shared http.Transport instead of allocating a new one each time.
func TestTransportReuse(t *testing.T) {
	var newConnCount atomic.Int32

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnCount.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	req := &Request{
		Name:      "transport-reuse-test",
		Method:    "GET",
		Url:       server.URL,
		Codes:     []int{200},
		Sensitive: false,
	}

	status1, err := req.Run()
	assert.NoError(t, err)
	assert.True(t, status1.Healthy)

	time.Sleep(100 * time.Millisecond)
	connsBefore := newConnCount.Load()

	// With a shared transport, the idle connection is reused
	status2, err := req.Run()
	assert.NoError(t, err)
	assert.True(t, status2.Healthy)

	time.Sleep(100 * time.Millisecond)
	connsAfter := newConnCount.Load()

	assert.Equal(t, connsBefore, connsAfter,
		"Second Run() opened a new connection (%d -> %d); transport is not being reused",
		connsBefore, connsAfter)
}

// TestSensitiveRequestOmitsBody verifies that sensitive requests never expose
// response body content in the returned status, regardless of content type.
func TestSensitiveRequestOmitsBody(t *testing.T) {
	contentTypes := []string{
		"application/json",
		"text/plain",
		"text/html",
		"application/xml",
	}

	for _, ct := range contentTypes {
		t.Run(ct, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", ct)
				w.WriteHeader(http.StatusOK)
				if ct == "application/json" {
					w.Write([]byte(`{"key": "secret-value"}`))
				} else {
					w.Write([]byte("secret body content"))
				}
			}))
			defer server.Close()

			req := &Request{
				Name:      "sensitive-omit-test",
				Method:    "GET",
				Url:       server.URL,
				Codes:     []int{200},
				Sensitive: true,
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.True(t, status.Healthy)
			assert.Nil(t, status.Response, "Sensitive response should have nil Response for %s", ct)
		})
	}
}

// TestSensitiveRequestOmitsBodyAllStatusCodes verifies body omission across
// various HTTP status codes.
func TestSensitiveRequestOmitsBodyAllStatusCodes(t *testing.T) {
	for _, code := range []int{200, 201, 204, 400, 500} {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				if code != 204 {
					w.Write([]byte(`{"secret":"data"}`))
				}
			}))
			defer server.Close()

			req := &Request{
				Name:      "sensitive-status-test",
				Method:    "GET",
				Url:       server.URL,
				Codes:     []int{code},
				Sensitive: true,
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.Nil(t, status.Response, "Sensitive response should have nil Response for status %d", code)
		})
	}
}

// TestJSONBodyDecoding verifies that non-sensitive requests with JSON responses
// decode the body into status.Response.
func TestJSONBodyDecoding(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		checkKey string
		checkVal interface{}
	}{
		{"simple", `{"status":"ok","count":42}`, "status", "ok"},
		{"nested", `{"data":{"nested":true}}`, "data", map[string]interface{}{"nested": true}},
		{"string", `{"message":"hello world"}`, "message", "hello world"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()

			req := &Request{
				Name:   "json-decode-test",
				Method: "GET",
				Url:    server.URL,
				Codes:  []int{200},
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.NotNil(t, status.Response)
			assert.Equal(t, tc.checkVal, status.Response[tc.checkKey])
		})
	}
}

// TestJSONContentTypeVariants verifies JSON decoding works with charset params.
func TestJSONContentTypeVariants(t *testing.T) {
	for _, ct := range []string{
		"application/json",
		"application/json; charset=utf-8",
		"application/json; charset=UTF-8",
	} {
		t.Run(ct, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", ct)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"result":"success"}`))
			}))
			defer server.Close()

			req := &Request{
				Name:   "json-variant-test",
				Method: "GET",
				Url:    server.URL,
				Codes:  []int{200},
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.NotNil(t, status.Response)
			assert.Equal(t, "success", status.Response["result"])
		})
	}
}

// TestEmptyJSONBody verifies that an empty JSON object decodes to an empty map.
func TestEmptyJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	req := &Request{
		Name:   "empty-json-test",
		Method: "GET",
		Url:    server.URL,
		Codes:  []int{200},
	}

	status, err := req.Run()
	assert.NoError(t, err)

	expected, _ := json.Marshal(map[string]interface{}{})
	actual, _ := json.Marshal(status.Response)
	assert.Equal(t, string(expected), string(actual))
}

// TestNonJSONBodyReadAsString verifies that non-JSON responses are stored as
// raw strings in status.Response["Body"].
func TestNonJSONBodyReadAsString(t *testing.T) {
	cases := []struct {
		contentType string
		body        string
	}{
		{"text/plain", "plain text response"},
		{"text/html", "<html><body>hello</body></html>"},
		{"application/xml", "<root><item>value</item></root>"},
		{"text/csv", "a,b,c\n1,2,3"},
	}

	for _, tc := range cases {
		t.Run(tc.contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()

			req := &Request{
				Name:   "non-json-body-test",
				Method: "GET",
				Url:    server.URL,
				Codes:  []int{200},
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.NotNil(t, status.Response)
			assert.Equal(t, tc.body, status.Response["Body"])
		})
	}
}

// TestHealthStatusDetermination verifies Healthy is true iff the response
// status code is in the configured Codes list.
func TestHealthStatusDetermination(t *testing.T) {
	cases := []struct {
		name       string
		serverCode int
		reqCodes   []int
		wantHealth bool
	}{
		{"200-in-[200]", 200, []int{200}, true},
		{"200-in-[200,201]", 200, []int{200, 201}, true},
		{"201-in-[200,201]", 201, []int{200, 201}, true},
		{"500-not-in-[200]", 500, []int{200}, false},
		{"404-not-in-[200]", 404, []int{200}, false},
		{"204-in-[204]", 204, []int{204}, true},
		{"200-not-in-[201,202]", 200, []int{201, 202}, false},
		{"503-in-[500,502,503]", 503, []int{500, 502, 503}, true},
		{"200-default-codes", 200, nil, true},
		{"404-default-codes", 404, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.serverCode)
				w.Write([]byte("ok"))
			}))
			defer server.Close()

			req := &Request{
				Name:   "health-test",
				Method: "GET",
				Url:    server.URL,
				Codes:  tc.reqCodes,
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.Equal(t, tc.wantHealth, status.Healthy,
				"Healthy should be %v for status %d with codes %v",
				tc.wantHealth, tc.serverCode, tc.reqCodes)
		})
	}
}

// TestStatusCodeAndStatusMatch verifies StatusCode and Status string reflect
// the actual HTTP response.
func TestStatusCodeAndStatusMatch(t *testing.T) {
	for _, code := range []int{200, 201, 204, 301, 400, 403, 404, 500, 502, 503} {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				w.Write([]byte("response"))
			}))
			defer server.Close()

			req := &Request{
				Name:   "status-match-test",
				Method: "GET",
				Url:    server.URL,
				Codes:  []int{code},
			}

			status, err := req.Run()
			assert.NoError(t, err)
			assert.Equal(t, code, status.StatusCode)
			assert.Contains(t, status.Status, fmt.Sprintf("%d", code))
		})
	}
}

// TestHealthStatusProperty uses testing/quick to fuzz health status determination
// across random status codes and code lists.
func TestHealthStatusProperty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := 200
		fmt.Sscanf(r.URL.Query().Get("code"), "%d", &code)
		w.WriteHeader(code)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		statusCode := 200 + rng.Intn(400)

		numCodes := 1 + rng.Intn(5)
		codes := make([]int, numCodes)
		for i := range codes {
			codes[i] = 200 + rng.Intn(400)
		}

		expectedHealthy := false
		for _, c := range codes {
			if c == statusCode {
				expectedHealthy = true
				break
			}
		}

		req := &Request{
			Name:   "health-prop-test",
			Method: "GET",
			Url:    fmt.Sprintf("%s?code=%d", server.URL, statusCode),
			Codes:  codes,
		}

		status, err := req.Run()
		if err != nil {
			return false
		}
		return status.Healthy == expectedHealthy
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Health status property violated: %v", err)
	}
}

// TestBodyParsingProperty uses testing/quick to fuzz body parsing across
// random content types — JSON bodies are decoded, non-JSON are stored as strings.
func TestBodyParsingProperty(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		isJSON := rng.Intn(2) == 0

		var contentType, body string
		var check func(RequestStatus) bool

		if isJSON {
			contentType = "application/json"
			val := fmt.Sprintf("value-%d", rng.Intn(10000))
			body = fmt.Sprintf(`{"key":"%s"}`, val)
			check = func(s RequestStatus) bool {
				return s.Response != nil && s.Response["key"] == val
			}
		} else {
			types := []string{"text/plain", "text/html", "application/xml", "text/csv"}
			contentType = types[rng.Intn(len(types))]
			body = fmt.Sprintf("body-%d", rng.Intn(10000))
			check = func(s RequestStatus) bool {
				return s.Response != nil && s.Response["Body"] == body
			}
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(body))
		}))
		defer server.Close()

		req := &Request{
			Name:   "body-prop-test",
			Method: "GET",
			Url:    server.URL,
			Codes:  []int{200},
		}

		status, err := req.Run()
		if err != nil {
			return false
		}
		return check(status)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 30}); err != nil {
		t.Errorf("Body parsing property violated: %v", err)
	}
}

// TestNameAndTimestamp verifies that status.Name and Timestamp are populated.
func TestNameAndTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	req := &Request{
		Name:   "name-timestamp-test",
		Method: "GET",
		Url:    server.URL,
		Codes:  []int{200},
	}

	before := time.Now()
	status, err := req.Run()
	after := time.Now()

	assert.NoError(t, err)
	assert.Equal(t, "name-timestamp-test", status.Name)
	assert.False(t, status.Timestamp.IsZero())
	assert.True(t, !status.Timestamp.Before(before) && !status.Timestamp.After(after))
}

// TestGetCodesDefault verifies GetCodes() returns [200] when Codes is nil.
func TestGetCodesDefault(t *testing.T) {
	req := &Request{Name: "default-codes"}
	assert.Equal(t, []int{200}, req.GetCodes())

	req2 := &Request{Name: "custom-codes", Codes: []int{201, 204}}
	assert.Equal(t, []int{201, 204}, req2.GetCodes())
}
