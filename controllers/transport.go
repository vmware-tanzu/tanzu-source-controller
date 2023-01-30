/*
Copyright 2022 VMware, Inc.

This product is licensed to you under the Apache License, V2.0 (the "License"). You may not use this product except in compliance with the License.

This product may include a number of subcomponents with separate copyright notices and license terms. Your use of these subcomponents is subject to the terms and conditions of the subcomponent's license, as noted in the LICENSE file.
*/

package controllers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Cert encapsulates loading PEM encoded bytes for a certificate. Each Cert should use one of the
// available options: a file system path, an x509 certificate, or raw PEM encoded bytes
type Cert struct {
	Path        string
	Certificate *x509.Certificate
	Raw         []byte
}

func (c *Cert) Bytes() ([]byte, error) {
	if c.Path != "" {
		return os.ReadFile(c.Path)
	}
	if c.Certificate != nil {
		raw := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: c.Certificate.Raw,
		})
		return raw, nil
	}
	return c.Raw, nil
}

// newTransport constructs a new http transport combining the system cert pool with custom defined
// CA certs. The ggcr default transport is used as a foundation.
func newTransport(ctx context.Context, certs []Cert) (*http.Transport, error) {
	log := logr.FromContextOrDiscard(ctx)

	transport := remote.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		log.Error(err, "unable to load system cert pool")
		return nil, err
	}
	transport.TLSClientConfig.RootCAs = pool
	for _, cert := range certs {
		data, err := cert.Bytes()
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		if ok := transport.TLSClientConfig.RootCAs.AppendCertsFromPEM(data); !ok {
			return nil, fmt.Errorf("unable to load custom cert")
		}
	}
	return transport, nil
}
