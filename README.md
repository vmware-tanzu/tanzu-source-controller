# tanzu-source-controller

[![CI](https://github.com/vmware-tanzu/tanzu-source-controller/actions/workflows/ci.yaml/badge.svg)](https://github.com/vmware-tanzu/tanzu-source-controller/actions/workflows/ci.yaml)

The controller follows the spirit of the FluxCD Source Controller. An [`ImageRepository`](#imagerepository) resource is able to resolve source from the content of an image in an image registry.

- [Pre-requisites](#pre-requisites)
- [Install](#install)
  - [Releases](#releases)
  - [From Source](#from-source)
- [Developer Setup](#developer-setup)
- [Troubleshooting](#troubleshooting)
- [Reference Documentation](#reference-documentation)
  - [ImageRepository](#imagerepository)
  - [MavenArtifact](#mavenartifact)
- [License](#license)

## Pre-requisites

We require [Golang 1.19+](https://golang.org) to build the controller and deploy it. Internally, the `make` target will create a local version of [`ko`](https://github.com/google/ko) to build the controller, and [`kapp`](https://get-kapp.io) to deploy the controller to a cluster.

All 3 CLIs can be easily installed via brew:

```sh
brew tap vmware-tanzu/carvel && brew install imgpkg kbld kapp
```

This project requires access to a [container registry](https://docs.docker.com/registry/introduction/) for fetching image metadata. Therefore, it will not work for images that have bypassed a registry by loading directly into a local daemon.

Finally requires installing [cert-manager](https://cert-manager.io).

## Install

### Releases

[Built release bundles](https://github.com/vmware-tanzu/tanzu-source-controller/releases) are available with step by step [install instructions](./docs/installing-release.md), or you can build directly from source.

### From Source

1. Install [cert-manager](https://cert-manager.io):

    ```sh
    kapp deploy -a cert-manager -f https://github.com/jetstack/cert-manager/releases/download/v1.8.0/cert-manager.yaml
    ```

2. Optional: Trust additional certificate authorities certificate

    If a `ImageRepository` resource references an image in a registry whose certificate was not signed by a Public Certificate Authority (CA), a certificate error x509: certificate signed by unknown authority will occur while applying conventions. To trust additional certificate authorities include the PEM encoded CA certificates in a file and set following environment variable to the location of that file.

    ```sh
    CA_DATA=path/to/certfile # a PEM-encoded CA certificate
    ```

3. Build and install Source Controller:

    ```sh
    kapp deploy -a source-controller \
      -f <( \
        ko resolve -f <( \
          ytt \
            -f dist/source-controller.yaml \
            -f dist/package-overlay.yaml \
            --data-value-file ca_cert_data=${CA_DATA:-dist/ca.pem} \
          ) \
      )
    ```

   Note: you'll need to `export KO_DOCKER_REPO=${ACCESSIBLE_DOCKER_REPO}` such that `ko` can push to the repository and your cluster can pull from it. Visit [the ko README](https://github.com/google/ko#choose-destination) for more information.

To uninstall Source Controller:

```sh
kapp delete -a source-conntroller
```

---

## Reference Documentation

### ImageRepository

```yaml
---
apiVersion: source.apps.tanzu.vmware.com/v1alpha1
kind: ImageRepository
metadata:
  name: imagerepository-sample
spec:
  image: registry.example/image/repository:tag
  # optional fields
  interval: 5m
  imagePullSecrets: []
  serviceAccountName: default
```

`ImageRepository` resolves source code defined in an OCI image repository, exposing the resulting source artifact at a URL defined by `.status.artifact.url`.

The interval determines how often to check tagged images for changes. Setting this value too high will result in delays discovering new sources, while setting it to low may trigger a registry's rate limits.

Repository credentials may be defined as image pull secrets either referenced directly from the resources at `.spec.imagePullSecrets`, or attached to a service account referenced at `.spec.serviceAccountName`. The default service account name `"default"` is used if not otherwise specified. The default credential helpers for the registry are also used, for example, pulling from GCR on a GKE cluster.

### MavenArtifact

```yaml
---
apiVersion: source.apps.tanzu.vmware.com/v1alpha1
kind: MavenArtifact
metadata:
  name: mavenartifact-sample
spec:
  artifact:
    groupId: org.springframework.boot
    artifactId: spring-boot
    version: "2.7.0"
  repository:
    url: https://repo1.maven.org/maven2
  interval: 5m0s
  timeout: 1m0s
```

`MavenArtifact` resolves artifact from a Maven repository, exposing the resulting artifact at a URL defined by `.status.artifact.url`.

The `interval` determines how often to check artifact for changes. Setting this value too high will result in delays in discovering new sources, while setting it too low may trigger a repository's rate limits.

Repository credentials may be defined as secrets referenced from the resources at `.spec.repository.secretRef`. Secret referenced by `spec.repository.secretRef` is parsed as following:

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: auth-secret
type: Opaque
data:
  username: <BASE64>
  password: <BASE64>
  caFile:   <BASE64>   // PEM Encoded certificate data for Custom CA 
  certFile: <BASE64>   // PEM-encoded client certificate
  keyFile:  <BASE64>   // Private Key  
```

Maven supports a broad set of `version` syntax. Source Controller supports a strict subset of Maven's version syntax in order to ensure compatibility and avoid user confusion. The subset of supported syntax may grow over time, but will never expand past the syntax defined directly by Maven. This behavior means that we can use `mvn` as a reference implementation for artifact resolution.

Version support implemented in the following order:

1. pinned version - an exact match of a version in `maven-metadata.xml (versioning/versions/version)`

2. `RELEASE` - metaversion defined in `maven-metadata.xml (versioning/release)`

3. `*-SNAPSHOT` - the newest artifact for a snapshot version

4. `LATEST` - metaversion defined in `maven-metadata.xml (versioning/latest)`

5. version ranges - <https://maven.apache.org/enforcer/enforcer-rules/versionRanges.html>

**NOTE:** Pinned versions should be immutable, all other versions are dynamic and may change at any time. The `.spec.interval` defines how frequently to check for updated artifacts.

## Troubleshooting

For basic troubleshooting, please see the troubleshooting guide [here](./docs/troubleshooting.md).

## Developer Setup

For Source Controller development setup, please see the development guide [here](./docs/development.md)

## Contributing

The tanzu-source-controller project team welcomes contributions from the community. Before you start working with tanzu-source-controller, please read and sign our Contributor License Agreement CLA. If you wish to contribute code and you have not signed our contributor license agreement (CLA), our bot will prompt you to do so when you open a Pull Request. For any questions about the CLA process, please refer to our [FAQ](https://cla.vmware.com/faq).

## License

Copyright 2022 VMware Inc. All rights reserved
