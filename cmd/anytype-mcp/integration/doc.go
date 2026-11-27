// Package integration holds the forwarder's end-to-end test: the built
// anytype-mcp child over real pipes against a throwaway heart. It is
// behind the "integration" build tag because it builds cmd/grpcserver, and
// it does not build on Windows (heartboot manages the heart process with
// Unix process groups):
//
//	go test -tags integration ./cmd/anytype-mcp/integration
package integration
