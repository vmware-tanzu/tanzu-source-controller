package registry

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/kubernetes"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func TestNewAuthRegistryServer(t *testing.T) {
	reg_user := "user"
	reg_pwd := "pass"

	registry, registryHost, err := NewAuthRegistryServer(reg_user, reg_pwd)
	utilruntime.Must(err)
	defer registry.Close()

	var pullsecrets = []corev1.Secret{}
	dcj := fmt.Sprintf(`{"auths": {%q: {"username": %q, "password": %q}}}`, registryHost, reg_user, reg_pwd)
	secret := corev1.Secret{
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dcj),
		},
	}
	pullsecrets = append(pullsecrets, secret)
	keychain, err := kubernetes.NewFromPullSecrets(context.TODO(), pullsecrets)

	type args struct {
		username string
		password string
	}
	tests := []struct {
		name      string
		basicauth *authn.Basic
		keyauth   authn.Keychain
		tag_count int
		wantErr   bool
	}{
		{
			name: "auth test basic",
			basicauth: &authn.Basic{
				Username: reg_user,
				Password: reg_pwd,
			},
			tag_count: 1,
			wantErr:   false,
		},
		{
			name:      "auth with k8schain",
			keyauth:   keychain,
			tag_count: 1,
			wantErr:   false,
		},
	}

	helloImage := fmt.Sprintf("%s/%s", registryHost, "hello")
	utilruntime.Must(LoadImageWithAuth(registry, "fixtures/hello.tar", helloImage, reg_user, reg_pwd))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := name.NewRepository(registryHost + "/hello")
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAuthRegistryServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.keyauth != nil {
				tags, err := remote.List(repo, remote.WithAuthFromKeychain(tt.keyauth), remote.WithTransport(registry.Client().Transport))
				if (err != nil) != tt.wantErr {
					t.Errorf("NewAuthRegistryServer() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				got := len(tags)
				if got != tt.tag_count {
					t.Errorf("NewAuthRegistryServer() error = %v, want %v", got, tt.tag_count)
					return
				}
			}
			if tt.basicauth != nil {
				tags, err := remote.List(repo, remote.WithAuth(tt.basicauth), remote.WithTransport(registry.Client().Transport))
				if (err != nil) != tt.wantErr {
					t.Errorf("NewAuthRegistryServer() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				got := len(tags)
				if got != tt.tag_count {
					t.Errorf("NewAuthRegistryServer() error = %v, want %v", got, tt.tag_count)
					return
				}

			}

		})
	}
}
