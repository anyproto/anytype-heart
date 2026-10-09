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
	// Description replaces the document's description.
	Description string
	// CallerRetryKey exposes idempotency_key to the caller. The executor
	// mints a key for every write anyway; this is for the call whose blind
	// replay is not harmless — toggling a reaction twice removes it.
	CallerRetryKey bool
	// RenameBodyMember gives a body member (by its wire name) another
	// argument name — the fix a body/parameter name collision names; the
	// executor still sends it under its wire name. DropArg removes
	// arguments.
	RenameBodyMember map[string]string
	DropArg          []string
}

// Reasons shared by several exclusions.
const (
	reasonPairing = "pairing and key minting are the host's flow, not the model's"
	reasonStream  = "a server-sent event stream; the in-process transport buffers responses whole"
	reasonBinary  = "a binary or range response with no text rendering"
	reasonStatus  = "an ephemeral typing or presence signal for a live client UI, not an agent action"
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
	"publish_chat_status":   {Exclude: reasonStatus},

	"search_space":  {ReadOnly: true},
	"search_global": {ReadOnly: true},
	"validate":      {ReadOnly: true},

	"toggle_chat_reaction": {CallerRetryKey: true},

	// the document's form also offers multipart with a local file, which
	// the in-process transport does not send: the tool takes the url form
	"upload_file": {Description: "Upload a file. Stores a file fetched from a URL in the space. A source that refuses the fetch, or a URL that cannot be fetched, is a 400 naming /url. The id that comes back is the one file blocks, image blocks and icon_image values reference."},

	"patch_object": {Destructive: true},
	"update_type":  {Destructive: true},
}
