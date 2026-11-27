// Package contract holds the numbers and names the forwarder shares with
// heart's HTTP MCP delivery (core/api/server/mcp.go). It imports nothing
// from heart, so the forwarder stays small; a test in core/api/server pins
// these values against the server's own.
package contract

import "time"

const (
	// MaxMessageBytes bounds one inbound JSON-RPC line (heart's
	// wrapper.MCPMaxMessageBytes).
	MaxMessageBytes = 8 << 20
	// MaxReplyBytes bounds a successful upstream reply body.
	MaxReplyBytes = 128 << 20
	// MaxErrorBodyBytes bounds what is read of a non-200 upstream body.
	MaxErrorBodyBytes = 64 << 10

	// Workers is how many exchanges run at once; heart refuses the ninth
	// call in flight per key.
	Workers = 4
	// QueueSize is how many accepted messages may wait for a worker, and
	// how many replies may wait for stdout.
	QueueSize = 32

	SessionHeader  = "Mcp-Session-Id"
	ProtocolHeader = "MCP-Protocol-Version"

	// RefreshAfter is how old the last read of the item may be before a
	// request re-reads it.
	RefreshAfter = 5 * time.Second
	// ExchangeTimeout bounds one upstream exchange including its body.
	ExchangeTimeout = 120 * time.Second
	// ShutdownDeadline bounds everything between the shutdown trigger and
	// exit: the drain, stdout and one DELETE of a known session.
	ShutdownDeadline = 10 * time.Second
	// StdoutStall is how long stdout may accept nothing before it counts as
	// broken.
	StdoutStall = 30 * time.Second
)

// HeartVersions are the protocol revisions heart's HTTP delivery answers,
// oldest first.
var HeartVersions = []string{"2025-06-18", "2025-11-25"}

// ClientVersions are the revisions the forwarder answers to its stdio
// client, oldest first. 2025-03-26 is not among them: it allows batches,
// which heart refuses.
var ClientVersions = []string{"2024-11-05", "2025-06-18", "2025-11-25"}

// Tiers are the route segments of /mcp/<tier>.
var Tiers = []string{"small", "large", "full"}
