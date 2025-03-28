
# Suppress kapp prompts with KAPP_ARGS="--yes"
KAPP_ARGS ?= "--yes=false"
CA_DATA ?= dist/ca.pem

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: test

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: diegen controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	$(DIEGEN) die:headerFile=./hack/boilerplate.go.txt paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run unit tests only.
	go test ./... -short -coverprofile cover.out

tidy: ## Run go mod tidy
	go mod tidy -v

##@ Build

run: tidy test ## Run a controller from your host.
	go run ./main.go

.PHONY: dist
dist: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	$(KUSTOMIZE) build config/default | $(YTT) -f - -f dist/strip-status.yaml > dist/source-controller.yaml

##@ Deployment
.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@echo "kapp deploy -a source-controller -n kube-system -f <(kustomize build config/crd)"
	@$(KAPP) deploy -a source-controller -n kube-system -f <($(KUSTOMIZE) build config/crd) $(KAPP_ARGS)

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	@echo "kapp delete -a source-controller -n kube-system"
	@$(KAPP) delete -a source-controller -n kube-system $(KAPP_ARGS)

.PHONY: deploy
deploy: test ## Deploy controller to the K8s cluster specified in ~/.kube/config. Optional CA_DATA=path/to/certfile # a PEM-encoded CA certificate
	@echo "kapp deploy -a source-controller -n kube-system -f <(ko resolve -f dist/source-controller.yaml)"
	@$(KAPP) deploy -a source-controller -n kube-system -f <($(KO) resolve -f <( $(YTT) -f dist/source-controller.yaml -f dist/package-overlay.yaml --data-value-file ca_cert_data=$(CA_DATA))) $(KAPP_ARGS)

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	@echo "kapp delete -a source-controller -n kube-system"
	@$(KAPP) delete -a source-controller -n kube-system $(KAPP_ARGS)



##@ Dev Tools

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

##@ Tool Binaries
KUBECTL ?= kubectl
YTT ?= $(LOCALBIN)/ytt
KCTRL ?= $(LOCALBIN)/kctrl
KAPP ?= $(LOCALBIN)/kapp
IMGPKG ?= $(LOCALBIN)/imgpkg
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
KO ?= $(LOCALBIN)/ko
DIEGEN ?= $(LOCALBIN)/diegen

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.17.2
GOLANGCI_LINT_VERSION ?= v1.61.0
KO_VERSION ?= 0.17.1
DIEGEN_VERSION=v0.15.0
GOOS ?= darwin

.PHONY: tools
tools: clean kustomize controller-gen golangci-lint carvel-tools ko-setup diegen ## Setup tools used in local development
	ls -al $(LOCALBIN)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

.PHONY: carvel-tools
carvel-tools: $(LOCALBIN) ## Downloads Carvel CLI tools locally
	if [[ ! -f $(YTT) ]]; then \
		curl -L https://carvel.dev/install.sh | K14SIO_INSTALL_BIN_DIR=$(LOCALBIN) bash; \
	fi

.PHONY: ko-setup
ko-setup: $(KO) ## Setup for ko binary
$(KO): $(LOCALBIN)
	@if [ ! -f $(KO) ]; then \
		echo curl -sSfL "https://github.com/ko-build/ko/releases/download/v$(KO_VERSION)/ko_$(KO_VERSION)_$(GOOS)_x86_64.tar.gz"; \
		curl -sSfL "https://github.com/ko-build/ko/releases/download/v$(KO_VERSION)/ko_$(KO_VERSION)_$(GOOS)_x86_64.tar.gz" > $(LOCALBIN)/ko.tar.gz; \
		tar xzf $(LOCALBIN)/ko.tar.gz -C $(LOCALBIN)/; \
		chmod +x $(LOCALBIN)/ko; \
	fi;

.PHONY: diegen
diegen: $(DIEGEN) ## Download die-gen locally
$(DIEGEN): $(LOCALBIN)
	@if [ ! -f $(DIEGEN) ]; then \
		echo "# installing $(@)"; \
		GOBIN=$(LOCALBIN) go install reconciler.io/dies/diegen@$(DIEGEN_VERSION); \
	fi;

.PHONY: clean
clean: ## Remove local downloaded and generated binaries
	rm -rf $(LOCALBIN)


# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef
