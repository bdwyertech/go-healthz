// Encoding: UTF-8

package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
)

type Proxy struct {
	Name    string   `yaml:"name"`
	Port    int      `yaml:"port"`
	Methods []string `yaml:"methods"`
	url     *url.URL
}

func (proxy *Proxy) Handler(p *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		target := proxy.URL()
		r.URL.Host = target.Host
		r.URL.Scheme = target.Scheme
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Host = target.Host

		r.URL.Path = mux.Vars(r)["rest"]
		p.ServeHTTP(w, r)
	}
}

func (p *Proxy) URL() *url.URL {
	if p.url != nil {
		return p.url
	}
	var err error
	p.url, err = url.Parse(fmt.Sprintf("http://127.0.0.1:%v", p.Port))
	if err != nil {
		log.Fatal(err)
	}
	return p.url
}
