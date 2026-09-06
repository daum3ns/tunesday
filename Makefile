SHELL := /bin/bash
BUILD-DIRECTORY := ./build

.PHONY: build-server test clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build-server:
	@mkdir -p $(BUILD-DIRECTORY)
	go build -ldflags "-X tunesday/tunesday.online/internal/web.Version=$(VERSION)" -o $(BUILD-DIRECTORY)/tunesday.online ./tunesday.online/cmd/server

test: build-server
	go clean -testcache
	go test -v ./...

clean:
	rm -f $(BUILD-DIRECTORY)/*
