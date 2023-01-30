# Installing Release Bundle

- [Installing Release Bundle](#installing-release-bundle)
  - [Preparation](#preparation)
    - [Container Registry](#container-registry)
    - [CLI Requirements](#cli-requirements)
  - [Installation](#installation)

## Preparation

### Container Registry

- Access to a [container registry](https://docs.docker.com/registry/introduction/) for fetching image metadata. It will not work for images that have bypassed a registry by loading directly into a local daemon.

### CLI Requirements

- [imgpkg](https://carvel.dev/imgpkg/) relocates a bundle into a local registry
- [kbld](https://carvel.dev/kbld/) rewrites image references in k8s yaml to pull from a relocated registry
- [kapp](https://carvel.dev/kapp/) for deploys and verifies k8s yaml

> All 3 CLIs can be easily installed via brew: `brew tap vmware-tanzu/carvel && brew install imgpkg kbld kapp`.

## Installation

Install [cert-manager](https://cert-manager.io).

```shell
kapp deploy -a cert-manager -f https://github.com/jetstack/cert-manager/releases/download/v1.8.0/cert-manager.yaml
```

Download your desired release from [the releases page](https://github.com/vmware-tanzu/tanzu-source-controller/releases).

To install the bundle, you'll first need to relocate it to a docker registry that you can access from your cluster.

```sh
SOURCE_CONTROLLER_VERSION=v0.0.0 # update with the release version downloaded
imgpkg copy --tar ~/Downloads/source-controller-bundle-${SOURCE_CONTROLLER_VERSION}.tar --to-repo ${DOCKER_REPO?:Required}/source-controller-bundle
```

Then, pull down the yaml you'll need for the installation and cd into the bundle:

```sh
rm -rf ./source-controller-bundle # imgpkg will create this for us
imgpkg pull -b ${DOCKER_REPO?:Required}/source-controller-bundle -o ./source-controller-bundle
```

Optional: Trust additional certificate authorities certificate

If a `ImageRepository` resource references an image in a registry whose certificate was not signed by a Public Certificate Authority (CA), a certificate error x509: certificate signed by unknown authority will occur while applying conventions. To trust additional certificate authorities include the PEM encoded CA certificates in a file and set following environment variable to the location of that file.

```sh
CA_DATA=path/to/certfile # a PEM-encoded CA certificate
```

With the images relocated and the unpacked bundle as your current working directory, deploy Source Controller:

```sh
kapp deploy -a source-controller -f <(ytt -f source-controller-bundle/config/source-controller.yaml -f source-controller-bundle/package-overlay.yaml --data-value-file ca_cert_data=${CA_DATA:-source-controller-bundle/ca.pem} | kbld -f source-controller-bundle/.imgpkg/images.yml -f -)
```

## Uninstall

To uninstall Source Controller:

```sh
kapp delete -a source-controller
```

---

[back](../README.md)
