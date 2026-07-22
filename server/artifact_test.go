/*
Copyright 2022 VMware, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewHTTPServer_SetsTimeouts(t *testing.T) {
	s := newHTTPServer("127.0.0.1:0", http.NewServeMux())

	if s.ReadHeaderTimeout != readHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", s.ReadHeaderTimeout, readHeaderTimeout)
	}
	if s.ReadTimeout != readTimeout {
		t.Errorf("ReadTimeout = %v, want %v", s.ReadTimeout, readTimeout)
	}
	if s.IdleTimeout != idleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", s.IdleTimeout, idleTimeout)
	}
	if s.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (unset, so large artifact downloads aren't truncated)", s.WriteTimeout)
	}
}

func TestStart_ServesFilesAndRejectsDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "artifact.tar.gz"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- New(addr, dir).Start(ctx) }()

	baseURL := "http://" + addr
	var resp *http.Response
	for range 50 {
		resp, err = http.Get(baseURL + "/artifact.tar.gz")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET file: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET file status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp, err = http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("GET directory: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET directory status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	cancel()
	if err := <-errCh; err != nil && err != http.ErrServerClosed {
		t.Errorf("Start() returned unexpected error: %v", err)
	}
}
