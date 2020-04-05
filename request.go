// Encoding: UTF-8

package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kofalt/go-memoize"
)

type Request struct {
	Name    string `yaml:"name"`
	Method  string `yaml:"method"`
	Url     string `yaml:"url"`
	Body    string `yaml:"body"`
	Headers []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"headers"`
	Codes     []int  `yaml:"codes"`
	Frequency string `yaml:"frequency"`
	Sensitive bool   `yaml:"sensitive"`
	cache     *memoize.Memoizer
}

type RequestStatus struct {
	Name       string
	Healthy    bool
	Error      error                  `json:",omitempty"`
	Response   map[string]interface{} `json:",omitempty"`
	Status     string                 `json:",omitempty"`
	StatusCode int                    `json:",omitempty"`
}

func (req *Request) Cache() (cache *memoize.Memoizer) {
	if req.cache != nil {
		return req.cache
	}

	duration := 5 * time.Second
	if req.Frequency != "" {
		var err error
		if duration, err = time.ParseDuration(req.Frequency); err != nil {
			log.Fatal(err)
		}
	}

	req.cache = memoize.NewMemoizer(duration, 5*time.Minute)

	return req.cache
}

func (req *Request) Status() (status RequestStatus, err error) {
	s, err, _ := req.Cache().Memoize(req.Name, func() (interface{}, error) {
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
	r, err := http.NewRequest(req.Method, req.Url, req.GetBody())
	if err != nil {
		log.Fatal(err)
	}

	for _, header := range req.Headers {
		r.Header.Add(header.Name, header.Value)
	}

	client := &http.Client{}

	resp, err := client.Do(r)
	if err != nil {
		status.Healthy = false
		status.Error = err
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

	if req.Sensitive || resp.Body == nil {
		return
	}

	defer resp.Body.Close()

	switch resp.Header.Get("Content-Type") {
	case "application/json":
		json.NewDecoder(resp.Body).Decode(&status.Response)
	default:
		status.Response = make(map[string]interface{})
		response, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			status.Response["BodyDecodeFailure"] = err
		} else {
			status.Response["Body"] = string(response)
		}
	}

	return
}

func (req *Request) GetBody() (body io.Reader) {
	if req.Method == "POST" {
		return strings.NewReader(req.Body)
	}
	return
}

func (req *Request) GetCodes() []int {
	if req.Codes != nil {
		return req.Codes
	}

	return []int{200}
}
