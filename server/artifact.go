/*
Copyright 2022 VMware, Inc.

This product is licensed to you under the Apache License, V2.0 (the "License"). You may not use this product except in compliance with the License.

This product may include a number of subcomponents with separate copyright notices and license terms. Your use of these subcomponents is subject to the terms and conditions of the subcomponent's license, as noted in the LICENSE file.
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
