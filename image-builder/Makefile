# /*
# Copyright 2022 SUSE.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#     http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# */

.DEFAULT_GOAL := help

RKE2_VERSION?=1.26.0+rke2r1

## --------------------------------------
## Help
## --------------------------------------

##@ Helpers
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-35s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Packer flags
## --------------------------------------

# Set Packer color to true if not already set in env variables
# Only valid for builds
ifneq (,$(findstring build-, $(MAKECMDGOALS)))
	# A build target
	PACKER_COLOR ?= true
	PACKER_FLAGS += -color=$(PACKER_COLOR)
endif

# If ON_ERROR_ASK=1 then Packer will set -on-error to ask, causing the Packer
# build to pause when any error happens, instead of simply exiting. This is
# useful when debugging unknown issues logging into the remote machine via ssh.
ifeq (1,$(strip $(ON_ERROR_ASK)))
	PACKER_FLAGS += -on-error=ask
endif

# If DEBUG=1 then Packer will set -debug, enabling debug mode for builds, providing
# more verbose logging
ifeq (1,$(strip $(DEBUG)))
	PACKER_FLAGS += -debug
endif

## --------------------------------------
## Platform and version combinations
## --------------------------------------

AWS_BUILD_NAMES ?= aws-opensuse-leap-155

## --------------------------------------
## Dynamic build targets
## --------------------------------------

AWS_BUILD_TARGETS := $(addprefix build-,$(AWS_BUILD_NAMES))

.PHONY: $(AWS_BUILD_TARGETS)
$(AWS_BUILD_TARGETS):
	packer build $(PACKER_FLAGS) -var-file="$(abspath aws/$(subst build-aws-,,$@).json)" -var "rke2_semver=$(RKE2_VERSION)" aws/packer.json

## --------------------------------------
## Document dynamic build targets
## --------------------------------------
##@ Builds
build-aws-opensuse-leap-155: ## Builds an AMI for OpenSUSE Leap 15.5
build-aws-all: $(AWS_BUILD_TARGETS) ## Builds all AMIs
