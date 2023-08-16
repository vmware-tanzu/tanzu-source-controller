
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Suppress kapp prompts with KAPP_ARGS="--yes"
KAPP_ARGS ?= "--yes=false"

# Tools
CONTROLLER_GEN ?= go run -modfile hack/go.mod sigs.k8s.io/controller-tools/cmd/controller-gen
DIEGEN ?= go run -modfile hack/go.mod dies.dev/diegen
GOIMPORTS ?= go run -modfile hack/go.mod golang.org/x/tools/cmd/goimports
KAPP ?= go run -modfile hack/go.mod github.com/vmware-tanzu/carvel-kapp/cmd/kapp
KO ?= go run -modfile hack/go.mod github.com/google/ko
KUSTOMIZE ?= go run -modfile hack/go.mod sigs.k8s.io/kustomize/kustomize/v5
YTT ?= go run -modfile hack/go.mod github.com/vmware-tanzu/carvel-ytt/cmd/ytt
WOKE ?= go run -modfile hack/go.mod github.com/get-woke/woke
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

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	@echo "controller-gen $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths=\"./...\" output:crd:artifacts:config=config/crd/bases"
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

	@echo "kustomize build config/default | ytt -f - -f dist/strip-status.yaml > dist/source-controller.yaml"
	@$(KUSTOMIZE) build config/default | $(YTT) -f - -f dist/strip-status.yaml > dist/source-controller.yaml

generate: ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	@echo "controller-gen object:headerFile=\"hack/boilerplate.go.txt\" paths=\"./...\""
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

	@echo "diegen die:headerFile=\"hack/boilerplate.go.txt\" paths=\"./...\""
	@$(DIEGEN) die:headerFile="hack/boilerplate.go.txt" paths="./..."

	@echo "find -L . -name 'zz_generated.*.go' -exec goimports --local github.com/vmware-tanzu/tanzu-source-controller -w {} \;"
	@find -L . -name 'zz_generated.*.go' -exec $(GOIMPORTS) --local github.com/vmware-tanzu/tanzu-source-controller -w {} \;

fmt: ## Run go fmt against code.
	@echo "goimports --local github.com/vmware-tanzu/tanzu-source-controller -w ."
	@$(GOIMPORTS) --local github.com/vmware-tanzu/tanzu-source-controller -w .

vet: ## Run go vet against code.
	go vet ./...

.PHONY: scan-terms
scan-terms: ## Scan for inclusive terminology
	@$(WOKE) . -c its-woke-rules.yaml --exit-1-on-failure

.PHONY: woke-rules 
woke-rules: # Downloads woke rules from https://via.vmw.com/its-woke-rules
	curl -L https://via.vmw.com/its-woke-rules -o its-woke-rules.yaml

test: manifests generate fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

##@ Build

run: test ## Run a controller from your host.
	go run ./main.go

##@ Deployment

install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@echo "kapp deploy -a source-controller -n kube-system -f <(kustomize build config/crd)"
	@$(KAPP) deploy -a source-controller -n kube-system -f <($(KUSTOMIZE) build config/crd) $(KAPP_ARGS)

uninstall: ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	@echo "kapp delete -a source-controller -n kube-system"
	@$(KAPP) delete -a source-controller -n kube-system $(KAPP_ARGS)

deploy: test ## Deploy controller to the K8s cluster specified in ~/.kube/config. Optional CA_DATA=path/to/certfile # a PEM-encoded CA certificate
	@echo "kapp deploy -a source-controller -n kube-system -f <(ko resolve -f dist/source-controller.yaml)"
	@$(KAPP) deploy -a source-controller -n kube-system -f <($(KO) resolve -f <( $(YTT) -f dist/source-controller.yaml -f dist/package-overlay.yaml --data-value-file ca_cert_data=$(CA_DATA))) $(KAPP_ARGS)

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	@echo "kapp delete -a source-controller -n kube-system"
	@$(KAPP) delete -a source-controller -n kube-system $(KAPP_ARGS)

tidy: ## Run go mod tidy
	go mod tidy -v
	cd hack; go mod tidy -v
