all: build
.PHONY: all

SOURCE_GIT_TAG ?=$(shell git describe --long --tags --abbrev=7 --match 'v[0-9]*' || echo 'v1.0.0-$(SOURCE_GIT_COMMIT)')
SOURCE_GIT_COMMIT ?=$(shell git rev-parse --short "HEAD^{commit}" 2>/dev/null)
IMAGE_TAG ?= latest

# OS_GIT_VERSION is populated by ART
# If building out of the ART pipeline, fallback to SOURCE_GIT_TAG
ifndef OS_GIT_VERSION
	OS_GIT_VERSION = $(SOURCE_GIT_TAG)
endif

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps.mk \
	targets/openshift/crd-schema-gen.mk \
)

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/...
GO_BUILD_FLAGS :=-tags strictfipsruntime

IMAGE_REGISTRY ?= quay.io/redhat-user-workloads/dynamicacceleratorsl-tenant
# Custom image references for each component, allowing overrides for development
OPERATOR_IMAGE ?= $(IMAGE_REGISTRY)/instaslice-operator:$(IMAGE_TAG)
DAEMONSET_IMAGE ?= $(IMAGE_REGISTRY)/instaslice-daemonset:$(IMAGE_TAG)
WEBHOOK_IMAGE ?= $(IMAGE_REGISTRY)/instaslice-webhook:$(IMAGE_TAG)

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
ifdef OSS
$(call build-image,instaslice-operator,$(OPERATOR_IMAGE), ./Dockerfile,.)
$(call build-image,instaslice-daemonset,$(DAEMONSET_IMAGE), ./Dockerfile.daemonset,.)
$(call build-image,instaslice-webhook,$(WEBHOOK_IMAGE), ./Dockerfile.webhook,.)

$(call verify-golang-versions,Dockerfile)
$(call verify-golang-versions,Dockerfile.daemonset)
else
$(call build-image,instaslice-operator,$(OPERATOR_IMAGE), ./Dockerfile.ocp,.)
$(call build-image,instaslice-daemonset,$(DAEMONSET_IMAGE), ./Dockerfile.daemonset.ocp,.)
$(call build-image,instaslice-webhook,$(WEBHOOK_IMAGE), ./Dockerfile.webhook.ocp,.)

$(call verify-golang-versions,Dockerfile.ocp)
$(call verify-golang-versions,Dockerfile.daemonset.ocp)
endif


regen-crd:
	go build -o _output/tools/bin/controller-gen ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen
	rm -f manifests/instaslice-operator.crd.yaml
	./_output/tools/bin/controller-gen crd paths=./pkg/apis/instasliceoperator/v1alpha1/... schemapatch:manifests=./manifests output:crd:dir=./manifests
	mv manifests/inference.redhat.com_instasliceoperators.yaml manifests/instaslice-operator.crd.yaml
	cp manifests/instaslice-operator.crd.yaml deploy/00_instaslice-operator.crd.yaml
	cp manifests/inference.redhat.com_instaslices.yaml deploy/00_instaslices.crd.yaml

build-images: update-manifests
	podman build -f Dockerfile.ocp -t ${OPERATOR_IMAGE} .
	podman push ${OPERATOR_IMAGE}
	podman build -f Dockerfile.daemonset.ocp -t ${DAEMONSET_IMAGE} .
	podman push ${DAEMONSET_IMAGE}
	podman build -f Dockerfile.webhook.ocp -t ${WEBHOOK_IMAGE} .
	podman push ${WEBHOOK_IMAGE}

generate: regen-crd generate-clients
.PHONY: generate

generate-clients:
	GO=GO111MODULE=on GOFLAGS=-mod=readonly hack/update-codegen.sh
.PHONY: generate-clients

verify-codegen:
	hack/verify-codegen.sh
.PHONY: verify-codegen

clean:
	$(RM) -r ./_tmp
.PHONY: clean

update-manifests:
	$(eval DEPLOYMENT_FILE := $(shell grep -l "RELATED_IMAGE_DAEMONSET_IMAGE\|RELATED_IMAGE_WEBHOOK_IMAGE" deploy/*.yaml))
	sed -i -E 's|^([[:space:]]*image:)[[:space:]].*|\1 $(OPERATOR_IMAGE)|' $(DEPLOYMENT_FILE)
	sed -i -E '/- name: RELATED_IMAGE_DAEMONSET_IMAGE/{n;s|^([[:space:]]*value:)[[:space:]].*|\1 $(DAEMONSET_IMAGE)|;}' $(DEPLOYMENT_FILE)
	sed -i -E '/- name: RELATED_IMAGE_WEBHOOK_IMAGE/{n;s|^([[:space:]]*value:)[[:space:]].*|\1 $(WEBHOOK_IMAGE)|;}' $(DEPLOYMENT_FILE)
.PHONY: update-manifests
