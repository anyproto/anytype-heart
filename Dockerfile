FROM golang:1.16-buster AS builder
MAINTAINER Anytype <dev@anytype.io>

# This is (in large part) copied (with love) from
# https://hub.docker.com/r/ipfs/go-ipfs/dockerfile

ENV SRC_DIR /anytype
ENV BUILD_DIR /tmp

# Download packages first so they can be cached.
COPY go.mod go.sum $SRC_DIR/
RUN cd $SRC_DIR \
  && go mod download

COPY . $SRC_DIR

# Install the binary
RUN cd $SRC_DIR \
  && go build -v -o $BUILD_DIR/server ./cmd/grpcserver/grpc.go

ENTRYPOINT ["/tmp/server"]
