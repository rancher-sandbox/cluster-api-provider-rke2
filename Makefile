# If you update this file, please follow
# https://www.thapaliya.com/en/writings/well-documented-makefiles/
#
# NOTE: the contents are based on upstream CAPI

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.DEFAULT_GOAL:=help

#
# Go.
#
GO_VERSION ?= 1.19.0
GO_CONTAINER_IMAGE ?= docker.io/library/golang:$(GO_VERSION)

# Use GOPROXY environment variable if set
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

#
# Kubebuilder.
#
export KUBEBUILDER_ENVTEST_KUBERNETES_VERSION ?= 1.25.0
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?= 60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?= 60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

# Directories
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BIN_DIR := bin
TEST_DIR := test
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/$(BIN_DIR))
CAPRKE2_DIR := controlplane
CAPBPR_DIR := bootstrap
E2E_DATA_DIR ?= $(ROOT_DIR)/test/e2e/data
E2E_CONF_FILE ?= $(ROOT_DIR)/test/e2e/config/e2e_conf.yaml
E2E_CONF_FILE_ENVSUBST := $(ROOT_DIR)/test/e2e/config/rke2-ci-envsubst.yaml

export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Set --output-base for conversion-gen if we are not within GOPATH
ifneq ($(abspath $(ROOT_DIR)),$(shell go env GOPATH)/src/github.com/rancher-sandbox/cluster-api-provider-rke2)
	CONVERSION_GEN_OUTPUT_BASE_CAPRKE2 := --output-base=$(ROOT_DIR)/$(CAPRKE2_DIR)
	CONVERSION_GEN_OUTPUT_BASE_CAPBPR := --output-base=$(ROOT_DIR)/$(CAPBPR_DIR)
else
	export GOPATH := $(shell go env GOPATH)
endif

GO_INSTALL := ./scripts/go-install.sh

# Binaries
KUSTOMIZE_VER := v4.5.2
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(abspath $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)-$(KUSTOMIZE_VER))
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v4

SETUP_ENVTEST_VER := v0.0.0-20211110210527-619e6b92dab9
SETUP_ENVTEST_BIN := setup-envtest
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/$(SETUP_ENVTEST_BIN)-$(SETUP_ENVTEST_VER))
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

CONTROLLER_GEN_VER := v0.10.0
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER))
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

CONVERSION_GEN_VER := v0.25.0
CONVERSION_GEN_BIN := conversion-gen
# We are intentionally using the binary without version suffix, to avoid the version
# in generated files.
CONVERSION_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_GEN_BIN))
CONVERSION_GEN_PKG := k8s.io/code-generator/cmd/conversion-gen

ENVSUBST_VER := v2.0.0-20210730161058-179042472c46
ENVSUBST_BIN := envsubst
ENVSUBST := $(abspath $(TOOLS_BIN_DIR)/$(ENVSUBST_BIN)-$(ENVSUBST_VER))
ENVSUBST_PKG := github.com/drone/envsubst/v2/cmd/envsubst

GO_APIDIFF_VER := v0.4.0
GO_APIDIFF_BIN := go-apidiff
GO_APIDIFF := $(abspath $(TOOLS_BIN_DIR)/$(GO_APIDIFF_BIN)-$(GO_APIDIFF_VER))
GO_APIDIFF_PKG := github.com/joelanford/go-apidiff

HADOLINT_VER := v2.10.0
HADOLINT_FAILURE_THRESHOLD = warning

GOLANGCI_LINT_VER := v1.49.0
GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN))

GH_VERSION := 2.7.0
GH_BIN := gh
GH := $(abspath $(TOOLS_BIN_DIR)/$(GH_BIN))

# Registry / images
TAG ?= dev
ARCH ?= $(shell go env GOARCH)
ALL_ARCH = amd64 arm arm64 ppc64le s390x
REGISTRY ?= ghcr.io
ORG ?= rancher-sandbox
CONTROLLER_IMAGE_NAME := cluster-api-provider-rke2
BOOTSTRAP_IMAGE_NAME := $(CONTROLLER_IMAGE_NAME)-bootstrap
CONTROLPLANE_IMAGE_NAME = $(CONTROLLER_IMAGE_NAME)-controlplane
BOOTSTRAP_IMG ?= $(REGISTRY)/$(ORG)/$(BOOTSTRAP_IMAGE_NAME)
CONTROLPLANE_IMG ?= $(REGISTRY)/$(ORG)/$(CONTROLPLANE_IMAGE_NAME)

# Repo
GH_ORG_NAME ?= $ORG
GH_REPO_NAME ?= cluster-api-provider-rke2
GH_REPO ?= $(GH_ORG_NAME)/$(GH_REPO_NAME)

# Set build time variables including version details
LDFLAGS := $(shell  hack/version.sh)

# Allow overriding the imagePullPolicy
PULL_POLICY ?= Always

.PHONY: all
all: test managers

help:  # Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-45s\033[0m %s\n", $$1, $$2 } /^\$$\([0-9A-Za-z_-]+\):.*?##/ { gsub("_","-", $$1); printf "  \033[36m%-45s\033[0m %s\n", tolower(substr($$1, 3, length($$1)-7)), $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Generate / Manifests
## --------------------------------------

##@ generate:

ALL_GENERATE_MODULES = rke2-bootstrap rke2-control-plane

.PHONY: generate
generate: ## Run all generate-manifests-*, generate-go-deepcopy-* targets
	$(MAKE) generate-modules generate-manifests generate-go-deepcopy

.PHONY: generate-manifests
generate-manifests: $(addprefix generate-manifests-,$(ALL_GENERATE_MODULES)) ## Run all generate-manifests-* targets

.PHONY: generate-manifests-rke2-bootstrap
generate-manifests-rke2-bootstrap: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for rke2 bootstrap provider
	$(MAKE) clean-generated-yaml SRC_DIRS="./bootstrap/config/crd/bases"
	$(CONTROLLER_GEN) \
		paths=./bootstrap/api/... \
		paths=./bootstrap/internal/controllers/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./bootstrap/config/crd/bases \
		output:rbac:dir=./bootstrap/config/rbac \
		output:webhook:dir=./bootstrap/config/webhook \
		webhook

.PHONY: generate-manifests-rke2-control-plane
generate-manifests-rke2-control-plane: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for RKE2 control plane provider
	$(MAKE) clean-generated-yaml SRC_DIRS="./controlplane/config/crd/bases"
	$(CONTROLLER_GEN) \
		paths=./controlplane/api/... \
		paths=./controlplane/internal/controllers/... \
		paths=./controlplane/internal/webhooks/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./controlplane/config/crd/bases \
		output:rbac:dir=./controlplane/config/rbac \
		output:webhook:dir=./controlplane/config/webhook \
		webhook

.PHONY: generate-go-deepcopy
generate-go-deepcopy:  ## Run all generate-go-deepcopy-* targets
	$(MAKE) $(addprefix generate-go-deepcopy-,$(ALL_GENERATE_MODULES))

.PHONY: generate-go-deepcopy-rke2-bootstrap
generate-go-deepcopy-rke2-bootstrap: $(CONTROLLER_GEN) ## Generate deepcopy go code for rke2 bootstrap
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./bootstrap/api"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate.go.txt \
		paths=./bootstrap/api/...

.PHONY: generate-go-deepcopy-rke2-control-plane
generate-go-deepcopy-rke2-control-plane: $(CONTROLLER_GEN) ## Generate deepcopy go code for rke2 control plane
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./controlplane/api"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate.go.txt \
		paths=./controlplane/api/...

.PHONY: generate-modules
generate-modules: ## Run go mod tidy to ensure modules are up to date
	go mod tidy
#	cd $(TOOLS_DIR); go mod tidy

## --------------------------------------
## Lint / Verify
## --------------------------------------

##@ lint and verify:

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Lint the codebase
	cd $(CAPBPR_DIR); $(GOLANGCI_LINT) run -v --timeout 3m $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(CAPRKE2_DIR); $(GOLANGCI_LINT) run -v --timeout 3m $(GOLANGCI_LINT_EXTRA_ARGS)
	./scripts/ci-lint-dockerfiles.sh $(HADOLINT_VER) $(HADOLINT_FAILURE_THRESHOLD)

.PHONY: lint-dockerfiles
lint-dockerfiles:
	./scripts/ci-lint-dockerfiles.sh $(HADOLINT_VER) $(HADOLINT_FAILURE_THRESHOLD)

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Lint the codebase and run auto-fixers if supported by the linter
	GOLANGCI_LINT_EXTRA_ARGS=--fix $(MAKE) lint

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

APIDIFF_OLD_COMMIT ?= $(shell git rev-parse origin/main)

.PHONY: apidiff
apidiff: $(GO_APIDIFF) ## Check for API differences
	$(GO_APIDIFF) $(APIDIFF_OLD_COMMIT) --print-compatible

ALL_VERIFY_CHECKS = modules gen

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS)) lint-dockerfiles ## Run all verify-* targets

.PHONY: verify-modules
verify-modules: generate-modules  ## Verify go modules are up to date
	@if !(git diff --quiet HEAD -- go.sum go.mod $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum $(TEST_DIR)/go.mod $(TEST_DIR)/go.sum); then \
		git diff; \
		echo "go module files are out of date"; exit 1; \
	fi
	@if (find . -name 'go.mod' | xargs -n1 grep -q -i 'k8s.io/client-go.*+incompatible'); then \
		find . -name "go.mod" -exec grep -i 'k8s.io/client-go.*+incompatible' {} \; -print; \
		echo "go module contains an incompatible client-go version"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate  ## Verify go generated files are up to date
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

## --------------------------------------
## Binaries
## --------------------------------------

##@ build:

ALL_MANAGERS = rke2-bootstrap rke2-control-plane

.PHONY: managers
managers: $(addprefix manager-,$(ALL_MANAGERS)) ## Run all manager-* targets

.PHONY: manager-rke2-bootstrap
manager-rke2-bootstrap: ## Build the rke2 bootstrap manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/rke2-bootstrap-manager github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap

.PHONY: manager-rke2-control-plane
manager-rke2-control-plane: ## Build the rke2 control plane manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/rke2-control-plane-manager github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane

.PHONY: docker-pull-prerequisites
docker-pull-prerequisites:
	docker pull docker.io/docker/dockerfile:1.4
	docker pull $(GO_CONTAINER_IMAGE)
	docker pull gcr.io/distroless/static:latest

.PHONY: docker-build-all
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH)) ## Build docker images for all architectures

docker-build-%:
	$(MAKE) ARCH=$* docker-build

ALL_DOCKER_BUILD = rke2-bootstrap rke2-control-plane

.PHONY: docker-build
docker-build: docker-pull-prerequisites ## Run docker-build-* targets for all providers
	$(MAKE) ARCH=$(ARCH) $(addprefix docker-build-,$(ALL_DOCKER_BUILD))

.PHONY: docker-build-rke2-bootstrap
docker-build-rke2-bootstrap: ## Build the docker image for rke2 bootstrap controller manager
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./bootstrap --build-arg ldflags="$(LDFLAGS)" . -t $(BOOTSTRAP_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(BOOTSTRAP_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-rke2-control-plane
docker-build-rke2-control-plane: ## Build the docker image for rke2 control plane controller manager
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./controlplane --build-arg ldflags="$(LDFLAGS)" . -t $(CONTROLPLANE_IMG)-$(ARCH):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLPLANE_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/config/default/manager_pull_policy.yaml"

## --------------------------------------
## Testing
## --------------------------------------

##@ test:

ARTIFACTS ?= ${ROOT_DIR}/_artifacts

KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))

.PHONY: test
test: $(SETUP_ENVTEST) ## Run unit and integration tests
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./... $(TEST_ARGS)

.PHONY: test-verbose
test-verbose: ## Run unit and integration tests with verbose flag
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-cover
test-cover: ## Run unit and integration tests and generate a coverage report
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -coverprofile=out/coverage.out"
	go tool cover -func=out/coverage.out -o out/coverage.txt
	go tool cover -html=out/coverage.out -o out/coverage.html

.PHONY: kind-cluster
kind-cluster: ## Create a new kind cluster designed for development with Tilt
	hack/kind-install-for-capd.sh

.PHONY: tilt-up
tilt-up: kind-cluster ## Start tilt and build kind cluster if needed.
	tilt up

## --------------------------------------
## E2E
## --------------------------------------

##@ e2e:

# Allow overriding the e2e configurations
GINKGO_FOCUS ?= Workload cluster creation
GINKGO_SKIP ?= API Version Upgrade
GINKGO_NODES ?= 1
GINKGO_NOCOLOR ?= false
GINKGO_ARGS ?=
GINKGO_TIMEOUT ?= 2h
GINKGO_POLL_PROGRESS_AFTER ?= 10m
GINKGO_POLL_PROGRESS_INTERVAL ?= 1m
ARTIFACTS ?= $(ROOT_DIR)/_artifacts
SKIP_CLEANUP ?= false
SKIP_CREATE_MGMT_CLUSTER ?= false

.PHONY: test-e2e-run
test-e2e-run: $(ENVSUBST) $(KUBECTL) $(GINKGO) e2e-image ## Run the end-to-end tests
	$(ENVSUBST) < $(E2E_CONF_FILE) > $(E2E_CONF_FILE_ENVSUBST) && \
	time $(GINKGO) -v --trace -poll-progress-after=$(GINKGO_POLL_PROGRESS_AFTER) -poll-progress-interval=$(GINKGO_POLL_PROGRESS_INTERVAL) \
	--tags=e2e --focus="$(GINKGO_FOCUS)" -skip="$(GINKGO_SKIP)" --nodes=$(GINKGO_NODES) --no-color=$(GINKGO_NOCOLOR) \
	--timeout=$(GINKGO_TIMEOUT) --output-dir="$(ARTIFACTS)" --junit-report="junit.e2e_suite.1.xml" $(GINKGO_ARGS) ./test/e2e -- \
		-e2e.artifacts-folder="$(ARTIFACTS)" \
		-e2e.config="$(E2E_CONF_FILE_ENVSUBST)" \
		-e2e.skip-resource-cleanup=$(SKIP_CLEANUP) \
		-e2e.use-existing-cluster=$(SKIP_CREATE_MGMT_CLUSTER) $(E2E_ARGS)

.PHONY: test-e2e
test-e2e: ## Run the end-to-end tests
	$(MAKE) test-e2e-run

LOCAL_GINKGO_ARGS ?=
LOCAL_GINKGO_ARGS += $(GINKGO_ARGS)
.PHONY: test-e2e-local
test-e2e-local: ## Run e2e tests
	PULL_POLICY=IfNotPresent MANAGER_IMAGE=$(CONTROLLER_IMG)-$(ARCH):$(TAG) \
	$(MAKE) docker-build \
	GINKGO_ARGS='$(LOCAL_GINKGO_ARGS)' \
	test-e2e-run

.PHONY: e2e-image
e2e-image:
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./controlplane --build-arg ldflags="$(LDFLAGS)" . -t $(CONTROLPLANE_IMG):$(TAG)
	DOCKER_BUILDKIT=1 docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg package=./bootstrap --build-arg ldflags="$(LDFLAGS)" . -t $(BOOTSTRAP_IMG):$(TAG)
	


## --------------------------------------
## Release
## --------------------------------------

##@ release:
## latest git tag for the commit, e.g., v0.3.10
RELEASE_TAG ?= $(shell git describe --abbrev=0 2>/dev/null)
ifneq (,$(findstring -,$(RELEASE_TAG)))
    PRE_RELEASE=true
endif
# the previous release tag, e.g., v0.3.9, excluding pre-release tags
PREVIOUS_TAG ?= $(shell git tag -l | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | sort -V | grep -B1 $(RELEASE_TAG) | head -n 1 2>/dev/null)
## set by Prow, ref name of the base branch, e.g., main
RELEASE_ALIAS_TAG := $(PULL_BASE_REF)
RELEASE_DIR := out
USER_FORK ?= $(shell git config --get remote.origin.url | cut -d/ -f4)
IMAGE_REVIEWERS ?= $(shell ./hack/get-project-maintainers.sh)

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: release
release: clean-release ## Build and push container images using the latest git tag for the commit
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "Your local git repository contains uncommitted changes, use git clean before proceeding."; exit 1; fi
	git checkout "${RELEASE_TAG}"
	# Build binaries first.
	# GIT_VERSION=$(RELEASE_TAG) $(MAKE) release-binaries
	# Set the manifest image to the production bucket.
	$(MAKE) manifest-modification REGISTRY=$(PROD_REGISTRY)
	## Build the manifests
	$(MAKE) release-manifests
	# Set the development manifest image to the staging bucket.
	# $(MAKE) manifest-modification-dev REGISTRY=$(STAGING_REGISTRY)
	## Build the development manifests
	# $(MAKE) release-manifests-dev
	## Clean the git artifacts modified in the release process
	$(MAKE) clean-release-git

.PHONY: manifest-modification
manifest-modification: # Set the manifest images to the staging/production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(BOOTSTRAP_IMG) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./bootstrap/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(CONTROLPLANE_IMG) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./controlplane/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./bootstrap/config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./controlplane/config/default/manager_pull_policy.yaml"

.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) $(KUSTOMIZE) ## Build the manifests to publish with a release
	# Build bootstrap-components.
	$(KUSTOMIZE) build bootstrap/config/default > $(RELEASE_DIR)/bootstrap-components.yaml
	# Build control-plane-components.
	$(KUSTOMIZE) build controlplane/config/default > $(RELEASE_DIR)/control-plane-components.yaml

	# Add metadata to the release artifacts
	cp metadata.yaml $(RELEASE_DIR)/metadata.yaml

.PHONY: release-notes
release-notes: $(RELEASE_DIR) $(GH)
	if [ -n "${PRE_RELEASE}" ]; then \
	echo ":rotating_light: This is a RELEASE CANDIDATE. Use it only for testing purposes. If you find any bugs, file an [issue](https://github.com/rancher-sandbox/cluster-api-provider-rke2/issues/new)." > $(RELEASE_DIR)/CHANGELOG.md; \
	else \
	$(GH) api repos/$(ORG)/$(GH_REPO_NAME)/releases/generate-notes -F tag_name=$(VERSION) -F previous_tag_name=$(PREVIOUS_VERSION) --jq '.body' > $(RELEASE_DIR)/CHANGELOG.md; \
	fi

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-push
docker-push: ## Push the docker images
	docker push $(BOOTSTRAP_IMG)-$(ARCH):$(TAG)
	docker push $(CONTROLPLANE_IMG)-$(ARCH):$(TAG)

.PHONY: docker-push-all
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))  ## Push all the architecture docker images
	$(MAKE) docker-push-manifest-rke2-bootstrap
	$(MAKE) docker-push-manifest-rke2-control-plane

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push-manifest-rke2-bootstrap
docker-push-manifest-rke2-bootstrap: ## Push the multiarch manifest for the rke2 bootstrap docker images
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(BOOTSTRAP_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(BOOTSTRAP_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${BOOTSTRAP_IMG}:${TAG} ${BOOTSTRAP_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(BOOTSTRAP_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(BOOTSTRAP_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/config/default/manager_pull_policy.yaml"

.PHONY: docker-push-manifest-rke2-control-plane
docker-push-manifest-rke-control-plane: ## Push the multiarch manifest for the rke2 control plane docker images
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(CONTROLPLANE_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLPLANE_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLPLANE_IMG}:${TAG} ${CONTROLPLANE_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge $(CONTROLPLANE_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(CONTROLPLANE_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/config/default/manager_pull_policy.yaml"

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for manager resources)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' $(TARGET_RESOURCE)

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for manager resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' $(TARGET_RESOURCE)

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

##@ clean:

.PHONY: clean
clean: ## Remove generated binaries and other build files.
	$(MAKE) clean-bin

.PHONY: clean-kind
clean-kind: ## Cleans up the kind cluster with the name $CAPI_KIND_CLUSTER_NAME
	kind delete cluster --name="$(CAPI_KIND_CLUSTER_NAME)" || true

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf $(BIN_DIR)
	rm -rf $(TOOLS_BIN_DIR)

.PHONY: clean-release
clean-release: ## Remove the release folder
	rm -rf $(RELEASE_DIR)

.PHONY: clean-manifests ## Reset manifests in config directories back to main
clean-manifests:
	@read -p "WARNING: This will reset all config directories to local main. Press [ENTER] to continue."
	git checkout main config $(CAPBPR_DIR)/config $(CAPRKE2_DIR)/config

.PHONY: clean-release-git
clean-release-git: ## Restores the git files usually modified during a release
	git restore ./*manager_image_patch.yaml ./*manager_pull_policy.yaml

.PHONY: clean-generated-yaml
clean-generated-yaml: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name '*.yaml' -exec rm -f {} \;; done)

.PHONY: clean-generated-deepcopy
clean-generated-deepcopy: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name 'zz_generated.deepcopy*' -exec rm -f {} \;; done)

## --------------------------------------
## Hack / Tools
## --------------------------------------

##@ hack/tools:

.PHONY: $(CONTROLLER_GEN_BIN)
$(CONTROLLER_GEN_BIN): $(CONTROLLER_GEN) ## Build a local copy of controller-gen.

.PHONY: $(CONVERSION_GEN_BIN)
$(CONVERSION_GEN_BIN): $(CONVERSION_GEN) ## Build a local copy of conversion-gen.

.PHONY: $(GO_APIDIFF_BIN)
$(GO_APIDIFF_BIN): $(GO_APIDIFF) ## Build a local copy of go-apidiff

.PHONY: $(ENVSUBST_BIN)
$(ENVSUBST_BIN): $(ENVSUBST) ## Build a local copy of envsubst.

.PHONY: $(KUSTOMIZE_BIN)
$(KUSTOMIZE_BIN): $(KUSTOMIZE) ## Build a local copy of kustomize.

.PHONY: $(SETUP_ENVTEST_BIN)
$(SETUP_ENVTEST_BIN): $(SETUP_ENVTEST) ## Build a local copy of setup-envtest.

.PHONY: $(GOLANGCI_LINT_BIN)
$(GOLANGCI_LINT_BIN): $(GOLANGCI_LINT) ## Build a local copy of golangci-lint

$(CONTROLLER_GEN): # Build controller-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)

.PHONY: $(CONVERSION_GEN)
$(CONVERSION_GEN): # Build conversion-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONVERSION_GEN_PKG) $(CONVERSION_GEN_BIN) $(CONVERSION_GEN_VER)

$(GO_APIDIFF): # Build go-apidiff from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GO_APIDIFF_PKG) $(GO_APIDIFF_BIN) $(GO_APIDIFF_VER)

$(ENVSUBST): # Build gotestsum from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(ENVSUBST_PKG) $(ENVSUBST_BIN) $(ENVSUBST_VER)

$(KUSTOMIZE): # Build kustomize from tools folder.
	CGO_ENABLED=0 GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(KUSTOMIZE_PKG) $(KUSTOMIZE_BIN) $(KUSTOMIZE_VER)

$(SETUP_ENVTEST): # Build setup-envtest from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(SETUP_ENVTEST_PKG) $(SETUP_ENVTEST_BIN) $(SETUP_ENVTEST_VER)

$(GOLANGCI_LINT): # Download and install golangci-lint
	hack/ensure-golangci-lint.sh \
		-b $(TOOLS_BIN_DIR) \
		$(GOLANGCI_LINT_VER)

$(GH): # Download GitHub cli into the tools bin folder
	hack/ensure-gh.sh \
		-b $(TOOLS_BIN_DIR) \
		$(GH_VERSION)