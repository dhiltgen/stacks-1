
ORG=dockereng
CONTROLLER_IMAGE_NAME=stack-controller
E2E_IMAGE_NAME=stack-e2e
TAG=latest # TODO work out versioning scheme
TEST_SCOPE?=./...
BUILD_ARGS= \
    --build-arg ALPINE_BASE=alpine:3.8 \
    --build-arg GOLANG_BASE=golang:1.11-alpine3.8

build: generate
	docker build $(BUILD_ARGS) -t $(ORG)/$(CONTROLLER_IMAGE_NAME):$(TAG) .

builder:
	docker build $(BUILD_ARGS) --target builder -t stacks-builder:$(TAG) .
	@echo ""
	@echo "To use the builder image, run 'docker run --rm -it stacks-builder:latest'"
	@echo ""

test:
	docker build $(BUILD_ARGS) -t $(ORG)/$(CONTROLLER_IMAGE_NAME):test --target unit-test .

lint:
	docker build $(BUILD_ARGS) -t $(ORG)/$(CONTROLLER_IMAGE_NAME):lint --target lint .

standalone:
	docker build $(BUILD_ARGS) -t $(ORG)/$(CONTROLLER_IMAGE_NAME):$(TAG) --target standalone .

e2e:
	docker build $(BUILD_ARGS) -t $(ORG)/$(E2E_IMAGE_NAME):$(TAG) --target e2e .

# For developers...


# Get coverage results in a web browser
cover: test
	docker create --name $(CONTROLLER_IMAGE_NAME)_cover $(ORG)/$(CONTROLLER_IMAGE_NAME):test  && \
	    docker cp $(CONTROLLER_IMAGE_NAME)_cover:/cover.out . && docker rm $(CONTROLLER_IMAGE_NAME)_cover
	go tool cover -html=cover.out

build-mocks:
	@echo "Generating mocks"
	mockgen -package=mocks github.com/docker/stacks/pkg/interfaces BackendClient | sed s,github.com/docker/stacks/vendor/,,g > pkg/mocks/mock_backend.go
	mockgen -package=mocks github.com/docker/stacks/pkg/reconciler/reconciler Reconciler | sed s,github.com/docker/stacks/vendor/,,g > pkg/mocks/mock_reconciler.go
	mockgen -package=mocks github.com/docker/stacks/pkg/store ResourcesClient | sed s,github.com/docker/stacks/vendor/,,g > pkg/mocks/mock_resources_client.go
	mockgen -package=mocks github.com/docker/compose-on-kubernetes/api/client/clientset/typed/compose/v1beta2 StackInterface,StacksGetter,ComposeV1beta2Interface | sed s,github.com/docker/stacks/vendor/,,g > pkg/mocks/mock_kubecompose_v1beta2.go
	mockgen -package=mocks k8s.io/client-go/kubernetes/typed/core/v1 CoreV1Interface,NamespaceInterface | sed s,github.com/docker/stacks/vendor/,,g > pkg/mocks/mock_kubernetes_corev1.go

generate: pkg/compose/schema/bindata.go

pkg/compose/schema/bindata.go: pkg/compose/schema/data/*.json
	docker build $(BUILD_ARGS) -t $(ORG)/$(CONTROLLER_IMAGE_NAME):build --target builder .
	docker create --name $(CONTROLLER_IMAGE_NAME)_schema $(ORG)/$(CONTROLLER_IMAGE_NAME):build && \
	    docker cp $(CONTROLLER_IMAGE_NAME)_schema:/go/src/github.com/docker/stacks/$@ $@ && docker rm $(CONTROLLER_IMAGE_NAME)_schema

.PHONY: e2e
