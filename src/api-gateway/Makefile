APP_NAME := api-gateway
IMAGE_NAME := $(APP_NAME)
TAG := latest

GOPATH:=$(shell go env GOPATH)

.PHONY: update
update:
	@go get -u

.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: build
build:
	@go build -o ./bin/$(APP_NAME) ./cmd/api-gateway

.PHONY: test
test:
	@go test -v ./... -cover

# Build the Docker image
.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) .

# Push the Docker image to Docker Hub
.PHONY: docker-push
docker-push:
	docker push $(IMAGE_NAME):$(TAG)