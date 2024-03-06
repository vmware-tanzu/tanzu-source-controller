package controllers

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/plainimage"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
)

type downloadError struct {
	err            error
	httpStatuscode int
}

func (d *downloadError) Error() string {
	return d.err.Error()
}

type MavenResolver struct {
	// Artifact data from Spec.Artifact.Type
	Artifact sourcev1alpha1.MavenArtifactType

	// Remote repository URL path where artifact is located
	RepositoryURL string

	// Request path
	RequestPath string

	// Resolved Vesrion of the artifact
	ResolvedVersion string

	// Resolved Filename of the artifact
	ResolvedFilename string

	// Download URL for the artifact from a remote repository. DownloadURL ends in ResolvedFileName
	DownloadURL string

	// Maven Metadata xml
	MetaXML string
}

func (r *MavenResolver) Resolve(ctx context.Context, client *http.Client) error {
	if strings.HasPrefix(r.Artifact.Version, "[") || strings.HasPrefix(r.Artifact.Version, "(") {
		return fmt.Errorf("Invalid version %q; ranges are not supported", r.Artifact.Version)
	}

	if r.Artifact.Version == "RELEASE" {
		return r.processReleaseVersion(ctx, client)
	}

	if r.Artifact.Version == "LATEST" {
		return r.processLatestVersion(ctx, client)
	}

	if strings.HasSuffix(r.Artifact.Version, "-SNAPSHOT") {
		return r.processSnapshotVersion(ctx, client)
	}

	return r.processFixedVersion()
}

func (r *MavenResolver) processFixedVersion() error {
	// update resolved artifact details
	r.ResolvedVersion = r.Artifact.Version

	// set artificat filename
	if len(r.Artifact.Classifier) > 0 {
		r.ResolvedFilename = fmt.Sprintf("%s-%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Classifier, r.Artifact.Type)
	} else {
		r.ResolvedFilename = fmt.Sprintf("%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Type)
	}

	// set artificat download URL
	r.DownloadURL = fmt.Sprintf("%s/%s/%s/%s", r.RepositoryURL, r.RequestPath, r.ResolvedVersion, r.ResolvedFilename)

	return nil
}

func (r *MavenResolver) processLatestVersion(ctx context.Context, client *http.Client) error {
	// set metadata URL
	metadataURL := fmt.Sprintf("%s/%s/%s", r.RepositoryURL, r.RequestPath, "maven-metadata.xml")

	// get metadata
	metadata, err := downloadMetadata(ctx, client, metadataURL)
	if err != nil {
		return err
	}

	// retrive latest version
	lv, err := metadata.LatestVersion()

	if err != nil {
		return err
	}

	metaxml, err := xml.MarshalIndent(metadata, "  ", "    ")
	if err != nil {
		return err
	}
	r.MetaXML = string(metaxml)

	// if latest version is a SNAPSHOT, process as SNAPSHOT
	if strings.HasSuffix(lv, "-SNAPSHOT") {
		r.Artifact.Version = lv
		return r.processSnapshotVersion(ctx, client)
	}

	// update artifact resolved version
	r.Artifact.Version = lv
	r.ResolvedVersion = lv

	// set artificat filename
	if len(r.Artifact.Classifier) > 0 {
		r.ResolvedFilename = fmt.Sprintf("%s-%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Classifier, r.Artifact.Type)
	} else {
		r.ResolvedFilename = fmt.Sprintf("%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Type)
	}

	// set artificat download URL
	r.DownloadURL = fmt.Sprintf("%s/%s/%s/%s", r.RepositoryURL, r.RequestPath, r.ResolvedVersion, r.ResolvedFilename)
	return nil
}

func (r *MavenResolver) processSnapshotVersion(ctx context.Context, client *http.Client) error {
	// set snapshot metadata URL
	metadataURL := fmt.Sprintf("%s/%s/%s/%s", r.RepositoryURL, r.RequestPath, r.Artifact.Version, "maven-metadata.xml")

	// get metadata
	metadata, err := downloadMetadata(ctx, client, metadataURL)
	if err != nil {
		return err
	}

	// if metadata contains snapshotVersions, resolve using snapshotVersions
	if len(metadata.Versioning.SnapshotVersions.SnapshotVersion) > 0 {
		r.ResolvedVersion = metadata.SnapshotResolvedVersion(r.Artifact.Type)
	} else {
		r.ResolvedVersion = r.Artifact.Version
	}

	// set artificat filename
	if len(r.Artifact.Classifier) > 0 {
		r.ResolvedFilename = fmt.Sprintf("%s-%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Classifier, r.Artifact.Type)
	} else {
		r.ResolvedFilename = fmt.Sprintf("%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Type)
	}

	// set artificat download URL
	r.DownloadURL = fmt.Sprintf("%s/%s/%s/%s", r.RepositoryURL, r.RequestPath, r.Artifact.Version, r.ResolvedFilename)
	return nil
}

func (r *MavenResolver) processReleaseVersion(ctx context.Context, client *http.Client) error {
	// set metadata URL
	metadataURL := fmt.Sprintf("%s/%s/%s", r.RepositoryURL, r.RequestPath, "maven-metadata.xml")

	// get metadata
	metadata, err := downloadMetadata(ctx, client, metadataURL)
	if err != nil {
		return err
	}

	// retrive release version
	rv, err := metadata.ReleaseVersion()
	if err != nil {
		return err
	}

	// update artifact details
	r.Artifact.Version = rv
	r.ResolvedVersion = rv

	// set artifact filename
	if len(r.Artifact.Classifier) > 0 {
		r.ResolvedFilename = fmt.Sprintf("%s-%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Classifier, r.Artifact.Type)
	} else {
		r.ResolvedFilename = fmt.Sprintf("%s-%s.%s", r.Artifact.ArtifactId, r.ResolvedVersion, r.Artifact.Type)
	}

	// set artifact download URL
	r.DownloadURL = fmt.Sprintf("%s/%s/%s/%s", r.RepositoryURL, r.RequestPath, r.ResolvedVersion, r.ResolvedFilename)
	return nil
}

// imgpkg logger

var _ plainimage.Logger = &NoopLogger{}

// NewNoopLogger creates a new noop logger
func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

// NoopLogger this logger will not print
type NoopLogger struct{}

// Logf does nothing
func (n NoopLogger) Logf(string, ...interface{}) {}
