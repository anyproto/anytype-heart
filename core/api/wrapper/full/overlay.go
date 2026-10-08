package full

// overlay.go — the hand-written part of the table (spec §2.3): per
// operationId, what the derivation must not do on its own. Every exclusion
// carries a reason, because the entry is the only way an operation can be
// absent; the conformance test refuses an operation that is neither derived
// nor listed here, and an overlay row for an operation the document no
// longer has.

// overlay is one row.
type overlay struct {
	// Exclude leaves the operation out, for the stated reason.
	Exclude string
	// ReadOnly and Destructive set the tool annotations where the HTTP
	// method does not say it: a POST that only reads, a PATCH that can
	// delete.
	ReadOnly    bool
	Destructive bool
	// OpsChannel names the op channel whose served schemas type the
	// operation's `ops` envelope.
	OpsChannel string
	// Description replaces the document's description.
	Description string
	// RenameArg renames arguments (the fix a body/parameter name collision
	// names); DropArg removes them.
	RenameArg map[string]string
	DropArg   []string
}

// Reasons shared by several exclusions.
const (
	reasonPairing = "pairing and key minting are the host's flow, not the model's"
	reasonStream  = "a server-sent event stream; the in-process transport buffers responses whole"
	reasonBinary  = "a binary or range response with no text rendering"
)

// overlays is the table, keyed by operationId.
var overlays = map[string]overlay{
	"auth_whoami":           {Exclude: reasonPairing},
	"create_auth_challenge": {Exclude: reasonPairing},
	"create_api_key":        {Exclude: reasonPairing},
	"stream_chat_messages":  {Exclude: reasonStream},
	"stream_space_chats":    {Exclude: reasonStream},
	"stream_space_search":   {Exclude: reasonStream},
	"head_file":             {Exclude: reasonBinary},
	"download_file":         {Exclude: reasonBinary},

	"search_space":  {ReadOnly: true},
	"search_global": {ReadOnly: true},
	"validate":      {ReadOnly: true},

	"patch_object": {Destructive: true, OpsChannel: "object"},
	"update_type":  {Destructive: true, OpsChannel: "type"},
}
