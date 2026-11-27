# anytype-mcp, the stdio forwarder AI apps start (cmd/anytype-mcp). Each target builds it twice:
#   <os-arch>-mcp-dev      the default build: the development namespace (items folder items-dev)
#   <os-arch>-mcp-release  the production namespace (items), fixed at link time; only release and
#                          nightly desktop builds pick this one
# All five are static, built without cgo, so any host builds any target.
# The binaries are not signed here: anytype-ts CI signs them with the app.
MCP_PACKAGE := github.com/anyproto/anytype-heart/cmd/anytype-mcp
MCP_LDFLAGS := -s -w -X main.version=$(or $(VERSION),dev)
MCP_RELEASE := -X main.linkNamespace=anytype-mcp-namespace:release
MCP_OS_ARCHS := darwin-amd64 darwin-arm64 windows-amd64 linux-amd64 linux-arm64

# $(1) the environment of the build, $(2) the os-arch
define mcp-build
	$(1) go build -trimpath -ldflags="$(MCP_LDFLAGS)" -o $(2)-mcp-dev $(MCP_PACKAGE)
	$(1) go build -trimpath -ldflags="$(MCP_LDFLAGS) $(MCP_RELEASE)" -o $(2)-mcp-release $(MCP_PACKAGE)
endef

MCP_DARWIN_ENV := GOOS=darwin CGO_ENABLED=0

cross-compile-mcp: $(addprefix cross-compile-mcp-,$(MCP_OS_ARCHS))

cross-compile-mcp-darwin-amd64:
	$(call mcp-build,$(MCP_DARWIN_ENV) GOARCH=amd64,darwin-amd64)

cross-compile-mcp-darwin-arm64:
	$(call mcp-build,$(MCP_DARWIN_ENV) GOARCH=arm64,darwin-arm64)

cross-compile-mcp-windows-amd64:
	$(call mcp-build,GOOS=windows GOARCH=amd64 CGO_ENABLED=0,windows-amd64)

cross-compile-mcp-linux-amd64:
	$(call mcp-build,GOOS=linux GOARCH=amd64 CGO_ENABLED=0,linux-amd64)

cross-compile-mcp-linux-arm64:
	$(call mcp-build,GOOS=linux GOARCH=arm64 CGO_ENABLED=0,linux-arm64)
