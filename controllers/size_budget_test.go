/*
Copyright 2026 VMware, Inc.

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

package controllers

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reconciler.io/runtime/reconcilers"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
)

func TestSizeLimitErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      *sizeLimitError
		expected string
	}{
		{
			name: "without recourse",
			err: &sizeLimitError{
				what:  `Maven artifact file "app.jar"`,
				limit: 1048576,
			},
			expected: `Maven artifact file "app.jar" exceeds the maximum allowed size of 1Mi`,
		},
		{
			name: "with recourse",
			err: &sizeLimitError{
				what:     `Maven artifact file "app.jar"`,
				limit:    1048576,
				recourse: maxArtifactSizeRecourse,
			},
			expected: `Maven artifact file "app.jar" exceeds the maximum allowed size of 1Mi` +
				`; raise the limit with the --maven-artifact-max-size flag (Carvel data value maven_artifact_max_size), or set it to "0" to disable the limit`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.expected {
				t.Errorf("Error() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestSizeBudgetUnderLimit(t *testing.T) {
	budget := newSizeBudget(100, "test data", "")
	var buf bytes.Buffer
	w := budget.wrap(&buf)

	n, err := w.Write(bytes.Repeat([]byte("a"), 50))
	if err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if n != 50 {
		t.Errorf("Write() = %d, want 50", n)
	}
	if buf.Len() != 50 {
		t.Errorf("buf.Len() = %d, want 50", buf.Len())
	}
}

func TestSizeBudgetExactlyAtLimit(t *testing.T) {
	budget := newSizeBudget(100, "test data", "")
	var buf bytes.Buffer
	w := budget.wrap(&buf)

	n, err := w.Write(bytes.Repeat([]byte("a"), 100))
	if err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if n != 100 {
		t.Errorf("Write() = %d, want 100", n)
	}
	if buf.Len() != 100 {
		t.Errorf("buf.Len() = %d, want 100", buf.Len())
	}
}

func TestSizeBudgetOneByteOver(t *testing.T) {
	budget := newSizeBudget(100, "test data", "")
	var buf bytes.Buffer
	w := budget.wrap(&buf)

	n, err := w.Write(bytes.Repeat([]byte("a"), 101))

	var sizeErr *sizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("Write() error = %v, want *sizeLimitError", err)
	}
	if sizeErr.limit != 100 {
		t.Errorf("sizeErr.limit = %d, want 100", sizeErr.limit)
	}
	if n != 0 {
		t.Errorf("Write() n = %d, want 0", n)
	}
	if buf.Len() != 0 {
		t.Errorf("buf.Len() = %d, want 0", buf.Len())
	}
}

func TestSizeBudgetRunningTotalAcrossWrites(t *testing.T) {
	budget := newSizeBudget(100, "test data", "")
	var buf bytes.Buffer
	w := budget.wrap(&buf)

	chunk := bytes.Repeat([]byte("a"), 40)

	if _, err := w.Write(chunk); err != nil {
		t.Fatalf("first Write() returned error: %v", err)
	}
	if _, err := w.Write(chunk); err != nil {
		t.Fatalf("second Write() returned error: %v", err)
	}

	_, err := w.Write(chunk)
	var sizeErr *sizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("third Write() error = %v, want *sizeLimitError", err)
	}
}

func TestSizeBudgetRunningTotalAcrossWrappedWriters(t *testing.T) {
	budget := newSizeBudget(100, "test data", "")
	var bufA, bufB bytes.Buffer

	if _, err := budget.wrap(&bufA).Write(bytes.Repeat([]byte("a"), 60)); err != nil {
		t.Fatalf("write to bufA returned error: %v", err)
	}

	_, err := budget.wrap(&bufB).Write(bytes.Repeat([]byte("a"), 60))
	var sizeErr *sizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("write to bufB error = %v, want *sizeLimitError", err)
	}
}

func TestSizeBudgetUnlimited(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
	}{
		{name: "zero", limit: 0},
		{name: "negative", limit: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			budget := newSizeBudget(tc.limit, "test data", "")
			var buf bytes.Buffer

			w := budget.wrap(&buf)
			if w != io.Writer(&buf) {
				t.Fatalf("wrap() returned a different io.Writer, want the same instance passed in")
			}

			payload := bytes.Repeat([]byte("a"), 10_000)
			n, err := w.Write(payload)
			if err != nil {
				t.Fatalf("Write() returned error: %v", err)
			}
			if n != len(payload) {
				t.Errorf("Write() = %d, want %d", n, len(payload))
			}
			if buf.Len() != len(payload) {
				t.Errorf("buf.Len() = %d, want %d", buf.Len(), len(payload))
			}
		})
	}
}

func TestDownloadOverBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("a"), 200))
	}))
	defer srv.Close()

	_, err := download(reconcilers.WithStash(context.Background()), srv.URL, srv.Client(), newSizeBudget(100, "test payload", ""))

	var sizeErr *sizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("download() error = %v, want *sizeLimitError", err)
	}
}

func TestDownloadAtBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("a"), 100))
	}))
	defer srv.Close()

	body, err := download(reconcilers.WithStash(context.Background()), srv.URL, srv.Client(), newSizeBudget(100, "test payload", ""))
	if err != nil {
		t.Fatalf("download() returned error: %v", err)
	}
	if len(body) != 100 {
		t.Errorf("len(body) = %d, want 100", len(body))
	}
}

func sha1Hex(body []byte) string {
	sum := sha1.Sum(body)
	return fmt.Sprintf("%x", sum)
}

func TestDownloadArtifactOverBudget(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 4*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	budget := newSizeBudget(1024, `Maven artifact file "test.jar"`, maxArtifactSizeRecourse)

	_, err := downloadArtifact(reconcilers.WithStash(context.Background()), srv.URL, dir, "test.jar", sha1Hex(body), srv.Client(), budget)

	var sizeErr *sizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("downloadArtifact() error = %v, want *sizeLimitError", err)
	}
	if !strings.Contains(err.Error(), "--maven-artifact-max-size") {
		t.Errorf("error message = %q, want it to contain %q", err.Error(), "--maven-artifact-max-size")
	}
}

func TestDownloadArtifactSizeErrorPrecedesChecksumMismatch(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 4*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	budget := newSizeBudget(1024, `Maven artifact file "test.jar"`, maxArtifactSizeRecourse)
	wrongChecksum := "0000000000000000000000000000000000000000"

	_, err := downloadArtifact(reconcilers.WithStash(context.Background()), srv.URL, dir, "test.jar", wrongChecksum, srv.Client(), budget)

	var sizeErr *sizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("downloadArtifact() error = %v, want *sizeLimitError", err)
	}
	if strings.Contains(err.Error(), "tampered with in transit") {
		t.Errorf("error message = %q, must not report a checksum tampering incident", err.Error())
	}
}

func TestDownloadArtifactAtBudget(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	budget := newSizeBudget(1024, `Maven artifact file "test.jar"`, maxArtifactSizeRecourse)

	artifactDir, err := downloadArtifact(reconcilers.WithStash(context.Background()), srv.URL, dir, "test.jar", sha1Hex(body), srv.Client(), budget)
	if err != nil {
		t.Fatalf("downloadArtifact() returned error: %v", err)
	}
	if artifactDir != filepath.Join(dir, "artifact") {
		t.Errorf("artifactDir = %q, want %q", artifactDir, filepath.Join(dir, "artifact"))
	}
}

func TestDownloadArtifactUnlimited(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 4*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	budget := newSizeBudget(0, `Maven artifact file "test.jar"`, "")

	artifactDir, err := downloadArtifact(reconcilers.WithStash(context.Background()), srv.URL, dir, "test.jar", sha1Hex(body), srv.Client(), budget)
	if err != nil {
		t.Fatalf("downloadArtifact() returned error: %v", err)
	}

	info, err := os.Stat(filepath.Join(artifactDir, "test.jar"))
	if err != nil {
		t.Fatalf("os.Stat() returned error: %v", err)
	}
	if info.Size() != 4096 {
		t.Errorf("file size = %d, want 4096", info.Size())
	}
}

// TestMavenArtifactDownloadSyncReconcilerBudgetIsPerSync guards against a
// budget captured by the closure returned from MavenArtifactDownloadSyncReconciler:
// that reconciler is a singleton shared across every MavenArtifact, so a
// closure-captured budget would leak its `remaining` count between resources
// instead of resetting each Sync.
func TestMavenArtifactDownloadSyncReconcilerBudgetIsPerSync(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 3*1024)
	checksum := sha1Hex(body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha1") {
			w.Write([]byte(checksum))
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	httpRootDir := t.TempDir()
	now := func() metav1.Time { return metav1.Time{Time: time.Unix(1, 0)} }

	reconciler := MavenArtifactDownloadSyncReconciler(httpRootDir, "artifact.example", now, 4096)
	syncReconciler, ok := reconciler.(*reconcilers.SyncReconciler[*sourcev1alpha1.MavenArtifact])
	if !ok {
		t.Fatalf("MavenArtifactDownloadSyncReconciler() returned %T, want *reconcilers.SyncReconciler", reconciler)
	}

	for _, name := range []string{"parent-a", "parent-b"} {
		parent := &sourcev1alpha1.MavenArtifact{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace", Name: name},
			Spec: sourcev1alpha1.MavenArtifactSpec{
				Timeout: &metav1.Duration{Duration: 5 * time.Minute},
			},
		}

		ctx := reconcilers.WithStash(context.Background())
		stashArtifactVersion(ctx, ArtifactDetails{
			ArtifactVersion:     "1.0",
			ResolvedFileName:    name + ".jar",
			ArtifactDownloadURL: srv.URL + "/" + name + ".jar",
		})
		stashHttpClient(ctx, srv.Client())

		if err := syncReconciler.Sync(ctx, parent); err != nil {
			t.Fatalf("Sync() for %s returned error: %v", name, err)
		}
		if parent.Status.Artifact == nil {
			t.Fatalf("Sync() for %s: expected Status.Artifact to be set", name)
		}
	}
}
