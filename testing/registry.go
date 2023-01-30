/*
Copyright 2022 VMware, Inc.

This product is licensed to you under the Apache License, V2.0 (the "License"). You may not use this product except in compliance with the License.

This product may include a number of subcomponents with separate copyright notices and license terms. Your use of these subcomponents is subject to the terms and conditions of the subcomponent's license, as noted in the LICENSE file.
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
