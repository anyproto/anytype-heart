FROM golang:1.20 AS builder
MAINTAINER Anytype <dev@anytype.io>

# This is (in large part) copied (with love) from
# https://hub.docker.com/r/ipfs/go-ipfs/dockerfile

WORKDIR /anytype

# Download packages first so they can be cached.
COPY go.mod go.sum /
ARG GITHUB_LOGIN
ARG GITHUB_TOKEN
RUN echo "machine github.com login $GITHUB_LOGIN password $GITHUB_TOKEN" >> ~/.netrc

RUN go mod download

COPY . .

# Install the binary
RUN go build -o server ./cmd/grpcserver/grpc.go

FROM ubuntu

# TODO: more fine-grained dependencies
RUN apt update && apt install -y curl
COPY --from=builder /anytype/server .
EXPOSE 31007
EXPOSE 31008
ENV ANYTYPE_GRPC_ADDR=:31007
ENV ANYTYPE_GRPCWEB_ADDR=:31008
CMD ["./server"]
