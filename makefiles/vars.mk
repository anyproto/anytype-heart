CUSTOM_NETWORK_FILE ?= ./core/anytype/config/nodes/custom.yml
OPENAPI_DOCS_DIR ?= ./core/api/docs
CLIENT_DESKTOP_PATH ?= ../anytype-ts
CLIENT_ANDROID_PATH ?= ../anytype-kotlin
CLIENT_IOS_PATH ?= ../anytype-swift
TANTIVY_GO_PATH ?= ../tantivy-go
BUILD_FLAGS ?=
TANTIVY_VERSION := $(shell cat go.mod | grep github.com/anyproto/tantivy-go | cut -d' ' -f2)

export GOLANGCI_LINT_VERSION=v2.2.1
export CGO_CFLAGS=-Wno-deprecated-non-prototype -Wno-unknown-warning-option -Wno-deprecated-declarations -Wno-xor-used-as-pow -Wno-single-bit-bitfield-constant-conversion


ifndef $(GOPATH)
GOPATH=$(shell go env GOPATH)
export GOPATH
endif

ifndef $(GOROOT)
GOROOT=$(shell go env GOROOT)
export GOROOT
endif

DEPS_PATH := $(shell pwd)/deps
export PATH := $(DEPS_PATH):$(PATH)
