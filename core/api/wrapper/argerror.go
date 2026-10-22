package wrapper

// ArgumentError is a pre-flight refusal: the call's shape was wrong — an
// unknown tool, an unknown or missing argument, a wrong type — and nothing
// reached the server. It is in-band on every delivery (the model reads the
// text and repairs the call), and distinct from a server refusal so a host
// budgeting repairs can tell the two apart.
type ArgumentError struct {
	Text string
}

func (e ArgumentError) Error() string { return e.Text }
