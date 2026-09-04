SHELL := /bin/bash
BUILD-DIRECTORY := ./build

.PHONY: build-server test clean

build-server:
	@mkdir -p $(BUILD-DIRECTORY)
	go build -o $(BUILD-DIRECTORY)/tunesday.online ./tunesday.online/cmd/server

test: build-server
	go clean -testcache
	go test -v ./...

clean:
	rm -f $(BUILD-DIRECTORY)/*
