# Development

## Install required dependencies

- Install [Golang 1.19+](https://golang.org)
- Install [Cert Manager](https://cert-manager.io)
- Install [Carvel Tools](https://carvel.dev)

*All 3 Carvel CLIs can be easily installed via brew*

```sh
brew tap vmware-tanzu/carvel && brew install imgpkg kbld kapp
```

## Build, Test and Install Source Controller

We build, test and install Source Controller via `make`. To see details:

```sh
make help
```

Note: you'll need to `export KO_DOCKER_REPO=${ACCESSIBLE_DOCKER_REPO}` such that `ko` can push to the repository and your cluster can pull from it. Visit [the ko README](https://github.com/google/ko#choose-destination) for more information.

To install Source Controller:

```sh
make deploy
```

To uninstall Source Controller:

```sh
make undeploy
```

---
[back](../README.md)
