package registry

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type AuthHandler struct {
	allowedUser, allowedPass string
	registryHandler          http.Handler
}

// ServeHTTP implements http.Handler
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.Header().Add("WWW-Authenticate", `Basic realm="Registry"`)
		w.WriteHeader(401)
		return
	}
	if !strings.HasPrefix(authHeader, "Basic ") {
		w.WriteHeader(403)
		w.Write([]byte(`Authorization header does not being with "Basic "`))
		return
	}
	namePass, err := base64.StdEncoding.DecodeString(authHeader[6:])
	if err != nil {
		w.WriteHeader(403)
		w.Write([]byte(`Authorization header doesn't appear to be base64-encoded`))
		return
	}
	namePassSlice := strings.SplitN(string(namePass), ":", 2)
	if len(namePassSlice) != 2 {
		w.WriteHeader(403)
		w.Write([]byte(`Authorization header doesn't appear to be colon-separated value `))
		w.Write(namePass)
		return
	}
	if namePassSlice[0] != h.allowedUser || namePassSlice[1] != h.allowedPass {
		w.WriteHeader(403)
		w.Write([]byte(`Authorization failed: wrong username or password`))
		return
	}
	h.registryHandler.ServeHTTP(w, r)
}

func NewAuthRegistryServer(username, password string) (*httptest.Server, string, error) {
	regHandler := registry.New()
	regHandler = &AuthHandler{
		registryHandler: regHandler,
		allowedUser:     username,
		allowedPass:     password,
	}

	tlsServer := httptest.NewTLSServer(regHandler)

	host, err := RegistryHost(tlsServer)

	return tlsServer, host, err
}

func LoadImageWithAuth(registry *httptest.Server, fromTar, toRepository string, username string, password string) error {
	tag, err := name.NewTag(toRepository, name.WeakValidation)
	if err != nil {
		return err
	}

	image, err := tarball.ImageFromPath(fromTar, nil)
	if err != nil {
		return err
	}

	return remote.Write(tag, image,
		remote.WithTransport(registry.Client().Transport),
		remote.WithAuth(&authn.Basic{
			Username: username,
			Password: password,
		}))
}
