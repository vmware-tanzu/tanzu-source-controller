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
	"archive/zip"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractArchiveRejectsZipSlip covers TNZ-103874: zip/jar entry names are
// attacker controlled (the archive is downloaded from a URL built from
// unvalidated MavenArtifact CR fields), so extractArchive must reject any
// entry whose name would resolve outside the destination directory rather
// than writing it there (zip-slip).
func TestExtractArchiveRejectsZipSlip(t *testing.T) {
	tests := []struct {
		name      string
		entryName string
	}{
		{
			name:      "parent directory traversal",
			entryName: "../../evil.txt",
		},
		{
			name:      "deep parent directory traversal",
			entryName: "../../../etc/evil.txt",
		},
		{
			name:      "absolute path",
			entryName: "/tmp/evil.txt",
		},
		{
			name:      "directory entry with traversal",
			entryName: "../evil-dir/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parentDir := t.TempDir()
			zipPath := writeTestZip(t, parentDir, tc.entryName, "pwned")

			if _, err := extractArchive(parentDir, zipPath); err == nil {
				t.Fatalf("expected extractArchive to reject entry %q, got nil error", tc.entryName)
			}

			escaped := filepath.Join(parentDir, filepath.Clean(tc.entryName))
			if _, err := os.Stat(escaped); !os.IsNotExist(err) {
				t.Fatalf("archive entry %q escaped destination directory: %q exists", tc.entryName, escaped)
			}
		})
	}
}

// TestExtractArchiveWellBehavedEntries is a regression guard: normal
// relative entries must still extract successfully after the zip-slip fix.
func TestExtractArchiveWellBehavedEntries(t *testing.T) {
	parentDir := t.TempDir()
	zipPath := writeTestZipMulti(t, parentDir, []zipEntry{
		{name: "META-INF/"},
		{name: "META-INF/MANIFEST.MF", contents: "Manifest-Version: 1.0\n"},
		{name: "com/"},
		{name: "com/example/"},
		{name: "com/example/Foo.class", contents: "fake-class-bytes"},
	})

	extractedDir, err := extractArchive(parentDir, zipPath)
	if err != nil {
		t.Fatalf("unexpected error extracting well-behaved archive: %v", err)
	}

	for _, entry := range []string{"META-INF/MANIFEST.MF", "com/example/Foo.class"} {
		if _, err := os.Stat(filepath.Join(extractedDir, entry)); err != nil {
			t.Fatalf("expected extracted file %q: %v", entry, err)
		}
	}
}

type zipEntry struct {
	name     string
	contents string
}

func writeTestZip(t *testing.T, dir, entryName, contents string) string {
	t.Helper()
	return writeTestZipMulti(t, dir, []zipEntry{{name: entryName, contents: contents}})
}

// writeTestZipMulti writes entries to a zip file in the given order. Order
// matters: extractArchive processes entries in zip order and does not create
// parent directories for a file entry, so a directory entry must precede any
// file entries nested under it (as real jar/zip archives do).
func writeTestZipMulti(t *testing.T, dir string, entries []zipEntry) string {
	t.Helper()

	zipPath := path.Join(dir, "test.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	for _, entry := range entries {
		w, err := zipWriter.Create(entry.name)
		if err != nil {
			t.Fatalf("failed to create zip entry %q: %v", entry.name, err)
		}
		// A directory entry (name ends in "/") has no content.
		if strings.HasSuffix(entry.name, "/") {
			continue
		}
		if _, err := w.Write([]byte(entry.contents)); err != nil {
			t.Fatalf("failed to write zip entry %q: %v", entry.name, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	return zipPath
}
