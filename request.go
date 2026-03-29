// Encoding: UTF-8

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/kofalt/go-memoize"
	"github.com/pkg/errors"
)

type Request struct {
	Name      string            `yaml:"name"`
	Method    string            `yaml:"method"`
	Url       string            `yaml:"url"`
	Body      string            `yaml:"body"`
	Headers   map[string]string `yaml:"headers"`
	Codes     []int             `yaml:"codes"`
	Cache     string            `yaml:"cache"`
	Timeout   string            `yaml:"timeout"`
	Sensitive bool              `yaml:"sensitive"`
	Insecure  bool              `yaml:"insecure"`
	cache     *memoize.Memoizer
	transport *http.Transport
}

type RequestStatus struct {
	Name       string
	Healthy    bool
	Error      string                 `json:",omitempty"`
	Response   map[string]interface{} `json:",omitempty"`
	Status     string                 `json:",omitempty"`
	StatusCode int                    `json:",omitempty"`
	Timestamp  time.Time
}

func (req *Request) Cached() (cache *memoize.Memoizer) {
	if req.cache != nil {
		return req.cache
	}

	duration := 5 * time.Second
	if req.Cache != "" {
		var err error
		if duration, err = time.ParseDuration(req.Cache); err != nil {
			log.Fatal(err)
		}
	}

	req.cache = memoize.NewMemoizer(duration, 5*time.Minute)

	return req.cache
}

func (req *Request) getTransport() *http.Transport {
	if req.transport != nil {
		return req.transport
	}

	transport := cleanhttp.DefaultPooledTransport()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: req.Insecure}
	req.transport = transport

	return req.transport
}

func (req *Request) Status() (status RequestStatus, err error) {
	s, err, _ := req.Cached().Memoize(req.Name, func() (interface{}, error) {
		return req.Run()
	})

	status, ok := s.(RequestStatus)
	if !ok {
		log.Fatal("Unable to convert response into RequestStatus")
	}
	return
}

func (req *Request) Run() (status RequestStatus, err error) {
	status.Name = req.Name
	defer func() { status.Timestamp = time.Now() }()

	timeout := 5 * time.Second
	if req.Timeout != "" {
		var err error
		if timeout, err = time.ParseDuration(req.Timeout); err != nil {
			log.Fatal(err)
		}
		if timeout > 20*time.Second {
			log.Warn(req.Name, ": Request timeout cannot be longer than 20 seconds!")
			timeout = 20 * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, req.Method, req.Url, req.GetBody())
	if err != nil {
		log.Fatal(err)
	}

	r.Header.Set("User-Agent", "go-healthz/"+ReleaseVer)
	for k, v := range req.Headers {
		r.Header.Set(k, v)
	}

	client := &http.Client{
		Transport: req.getTransport(),
	}

	log.Debugln("Executing Request:", req.Name)
	resp, err := client.Do(r)
	if err != nil {
		status.Healthy = false
		if ctxErr := ctx.Err(); errors.Is(err, context.DeadlineExceeded) {
			status.Error = errors.Wrap(ctxErr, "Request timed out").Error()
			log.Warnf("%v: %v", req.Name, status.Error)
		} else {
			status.Error = err.Error()
		}
		return
	}

	status.Status = resp.Status
	status.StatusCode = resp.StatusCode
	for _, code := range req.GetCodes() {
		if resp.StatusCode == code {
			status.Healthy = true
			break
		}
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if req.Sensitive || resp.Body == nil {
		// Drain the body so the underlying TCP connection can be reused
		if resp.Body != nil {
			io.Copy(io.Discard, resp.Body)
		}
		return
	}

	contentType := resp.Header.Get("Content-Type")
	switch strings.Split(contentType, ";")[0] {
	case "application/json":
		json.NewDecoder(resp.Body).Decode(&status.Response)
	default:
		status.Response = make(map[string]interface{})
		response, err := io.ReadAll(resp.Body)
		if err != nil {
			status.Response["BodyDecodeFailure"] = err
		} else {
			status.Response["Body"] = string(response)
		}
	}

	return
}

func (req *Request) GetBody() (body io.Reader) {
	if req.Method == http.MethodPost {
		return strings.NewReader(strings.TrimSpace(req.Body))
	}

	return
}

func (req *Request) GetCodes() []int {
	if req.Codes != nil {
		return req.Codes
	}

	return []int{200}
}
