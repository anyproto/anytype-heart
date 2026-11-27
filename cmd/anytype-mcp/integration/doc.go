// Package integration holds the forwarder's end-to-end test: the built
// anytype-mcp child over real pipes against a throwaway heart. It is
// behind the "integration" build tag because it builds cmd/grpcserver:
//
//	go test -tags integration ./cmd/anytype-mcp/integration
package integration
