GOPATH:=$(shell go env GOPATH)
PROTO_DIR=./proto
POST_SERVICE_PROTO_DIR=$(PROTO_DIR)/post-service

.PHONY: init
init:
	@go get -u google.golang.org/protobuf/proto
	@go install github.com/golang/protobuf/protoc-gen-go@latest
	@go install github.com/go-micro/generator/cmd/protoc-gen-micro@latest

.PHONY: proto
proto:
	@protoc --proto_path=./proto/post-service --micro_out=./proto/post-service --micro_opt=paths=source_relative --go_out=:./proto/post-service --go_opt=paths=source_relative post-service.proto


.PHONY: update
update:
	@go get -u

.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: build
build:
	@go build -o cmd/post-service/main.go *.go

.PHONY: test
test:
	@go test -v ./... -cover

.PHONY: docker
docker:
	@docker build -t api-gateway:latest .