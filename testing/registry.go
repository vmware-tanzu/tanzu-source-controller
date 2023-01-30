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

package registry

import (
	"net/http/httptest"
	"net/url"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func NewRegistry() (*httptest.Server, string, error) {
	registry, err := ggcrregistry.TLS("localhost")
	if err != nil {
		return nil, "", err
	}
	host, err := RegistryHost(registry)
	return registry, host, err
}

func RegistryHost(registry *httptest.Server) (string, error) {
	u, err := url.Parse(registry.URL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func LoadImage(registry *httptest.Server, fromTar, toRepository string) error {
	tag, err := name.NewTag(toRepository, name.WeakValidation)
	if err != nil {
		return err
	}

	image, err := tarball.ImageFromPath(fromTar, nil)
	if err != nil {
		return err
	}

	return remote.Write(tag, image, remote.WithTransport(registry.Client().Transport))
}
