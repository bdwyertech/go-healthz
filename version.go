// Encoding: UTF-8
//
// Go Healthz
//
// Copyright © 2020 Brian Dwyer - Intelligent Digital Services
//

package main

import (
	"flag"
	"fmt"
)

var versionFlag bool

func init() {
	flag.BoolVar(&versionFlag, "version", false, "Display version")
}

var GitCommit, ReleaseVer, ReleaseDate string

func showVersion() {
	if GitCommit == "" {
		GitCommit = "DEVELOPMENT"
	}
	if ReleaseVer == "" {
		ReleaseVer = "DEVELOPMENT"
	}
	fmt.Println("version:", ReleaseVer)
	fmt.Println("commit:", GitCommit)
	fmt.Println("date:", ReleaseDate)
}
