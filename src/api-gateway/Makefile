APP_NAME := api-gateway
IMAGE_NAME := $(APP_NAME)
TAG := latest

GOPATH:=$(shell go env GOPATH)

.PHONY: init
init:
	@go get -u google.golang.org/protobuf/proto
	@go install github.com/golang/protobuf/protoc-gen-go@latest
	@go install github.com/go-micro/generator/cmd/protoc-gen-micro@latest

.PHONY: proto
proto:
	@protoc --proto_path=./proto --micro_out=./proto --micro_opt=paths=source_relative --go_out=:./proto --go_opt=paths=source_relative post-service.proto

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