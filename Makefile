SHELL := /bin/bash
BUILD-DIRECTORY := ./build
BINARY := tunesday
BINARY-PATH := $(BUILD-DIRECTORY)/$(BINARY)

.PHONY: build

build:
	@mkdir -p $(BUILD-DIRECTORY)
	@rm -rf $(BINARY-PATH)
	go build -o $(BINARY-PATH) ./cmd/tunesday



test: build
	go clean -testcache
	go test -v ./...

clean:
	rm -f $(BUILD-DIRECTORY)/*
