all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps-gomod.mk \
	targets/openshift/images.mk \
	targets/openshift/bindata.mk \
)

# Run core verification and all self contained tests.
#
# Example:
#   make check
check: | verify test-unit
.PHONY: check

IMAGE_REGISTRY?=registry.svc.ci.openshift.org

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
# It will generate target "image-$(1)" for building the image and binding it as a prerequisite to target "images".
$(call build-image,ocp-vsphere-problem-detector,$(IMAGE_REGISTRY)/ocp/4.7:vsphere-problem-detector,./Dockerfile.rhel7,.)

clean:
	$(RM) vsphere-problem-detector
.PHONY: clean

GO_TEST_PACKAGES :=./pkg/... ./cmd/...

# Run e2e tests.
#
# Example:
#   make test-e2e
#test-e2e: GO_TEST_PACKAGES :=./test/e2e/...
#test-e2e: GO_TEST_FLAGS += -v
#test-e2e: test-unit
#.PHONY: test-e2e
