SHELL := /bin/bash
BUILD-DIRECTORY := ./build
BINARY := tunesday
BINARY-PATH := $(BUILD-DIRECTORY)/$(BINARY)

.PHONY: build build-server

build:
	@mkdir -p $(BUILD-DIRECTORY)
	@rm -rf $(BINARY-PATH)
	go build -o $(BINARY-PATH) ./cmd/tunesday

build-server:
	@mkdir -p $(BUILD-DIRECTORY)
	go build -o $(BUILD-DIRECTORY)/tunesday.online ./tunesday.online/cmd/server

test: build
	go clean -testcache
	go test -v ./...

clean:
	rm -f $(BUILD-DIRECTORY)/*
