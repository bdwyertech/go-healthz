package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefault(t *testing.T) {
	cwd, err := os.Getwd()
	assert.Nil(t, err)
	os.Args = []string{"cmd", "-config=" + filepath.Join(cwd, "test", "config.yml")}

	// Hack! :-)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		go main()
		<-ctx.Done()
	}()
	// Wait a second for the Server
	time.Sleep(1 * time.Second)

	resp, err := http.Get("http://localhost:3456")
	assert.Nil(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var status GlobalStatus

	err = json.Unmarshal(body, &status)
	assert.Nil(t, err, "Status should be a valid JSON GlobalStatus")

	assert.Equal(t, true, status.Healthy, "Global status should be healthy")
}
