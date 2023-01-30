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
	"net/http"
	"strings"
)

func New(addr string, dir string) *server {
	return &server{
		Addr: addr,
		Dir:  dir,
	}
}

type server struct {
	Addr string
	Dir  string
}

func (s *server) Start(ctx context.Context) error {
	directoryHandler := http.FileServer(http.Dir(s.Dir))
	server := http.Server{
		Addr: s.Addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/") {
				// deactivate directory listings
				// TODO deactivate redirects for directories `dir` -> `dir/`
				http.NotFound(w, r)
				return
			}
			directoryHandler.ServeHTTP(w, r)
		}),
	}

	// shutdown server when the context closes
	go func() {
		<-ctx.Done()
		server.Close()
	}()

	return server.ListenAndServe()
}
