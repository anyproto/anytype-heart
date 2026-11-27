// Package forward is the forwarder's transport: JSON-RPC lines on stdio,
// one HTTP exchange per message with heart's /mcp/<tier> on loopback.
package forward

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// JSON-RPC error codes the forwarder answers with.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInternal       = -32603
	codeBusy           = -32000
	codeNotConnected   = -32001
	codeNotInitialized = -32002
)

var nullID = json.RawMessage("null")

// reserved are the members of a JSON-RPC message. heart decodes into a Go
// struct, which matches member names case-insensitively, so a member that
// differs from one of these only by case would mean one thing here and
// another there. Such a frame is refused before anything looks at it.
var reserved = []string{"jsonrpc", "id", "method", "params", "result", "error"}

type kind int

const (
	kindRequest kind = iota
	kindNotification
	kindResponse
)

// message is one validated inbound line.
type message struct {
	kind   kind
	id     json.RawMessage // raw bytes, never re-encoded
	method string
	params json.RawMessage
	raw    []byte // the whole line, trimmed: the upstream body
}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// reply is one outbound line. before runs when the writer turns to the
// line, just before it is written. A reply with only flushed set is a
// marker: the writer closes it when every line before it has been written.
type reply struct {
	line    []byte
	before  func()
	flushed chan struct{}
}

type wireReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// encode builds the line. Marshal validates and compacts the raw members,
// so a reply is always one complete line of valid JSON.
func encode(w wireReply) (*reply, error) {
	w.JSONRPC = "2.0"
	if len(w.ID) == 0 {
		w.ID = nullID
	}
	line, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}
	return &reply{line: append(line, '\n')}, nil
}

// internalLine is the last resort when a reply can't be encoded.
const internalLine = `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}` + "\n"

func errorReply(id json.RawMessage, code int, msg string) *reply {
	body, err := json.Marshal(rpcError{Code: code, Message: msg})
	if err != nil {
		return &reply{line: []byte(internalLine)}
	}
	r, err := encode(wireReply{ID: id, Error: body})
	if err != nil {
		// the id was validated on the way in; this cannot happen
		return &reply{line: []byte(internalLine)}
	}
	return r
}

// toolErrorReply is a tools/call result with isError: the model reads the
// text and can act on it.
func toolErrorReply(id json.RawMessage, text string) *reply {
	result, err := json.Marshal(map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": true,
	})
	if err != nil {
		return errorReply(id, codeInternal, "internal error")
	}
	r, err := encode(wireReply{ID: id, Result: result})
	if err != nil {
		return errorReply(nullID, codeInternal, "internal error")
	}
	return r
}

func resultReply(id, result json.RawMessage) (*reply, error) {
	return encode(wireReply{ID: id, Result: result})
}

// errLineTooLong is a line over the limit; it has been skipped to its
// newline and the next line is intact.
var errLineTooLong = errors.New("line too long")

// readLine returns the next line without its newline. A line longer than
// limit is discarded to its newline and reported as errLineTooLong. At end
// of input it returns what is left with io.EOF.
func readLine(br *bufio.Reader, limit int) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > limit+1 {
				tooLong = true
				buf = nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		switch {
		case err == nil:
			if tooLong {
				return nil, errLineTooLong
			}
			buf = buf[:len(buf)-1]
			if len(buf) > limit {
				return nil, errLineTooLong
			}
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			if tooLong {
				return nil, errLineTooLong
			}
			if len(buf) > limit {
				return nil, errLineTooLong
			}
			return buf, err
		}
	}
}

func validID(id json.RawMessage) bool {
	t := bytes.TrimSpace(id)
	if len(t) == 0 {
		return false
	}
	switch t[0] {
	case '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return json.Valid(t)
	}
	return false
}

// parseEnvelope validates one line the way heart's classifier does
// (wrapper.parseMessage), plus the reserved-member rule. It returns the
// message, or the refusal to write.
func parseEnvelope(line []byte) (*message, *reply) {
	raw := bytes.TrimSpace(line)
	if len(raw) > 0 && raw[0] == '[' {
		return nil, errorReply(nullID, codeInvalidRequest, "JSON-RPC batching is not supported: send one message at a time")
	}
	if !json.Valid(raw) {
		return nil, errorReply(nullID, codeParse, "parse JSON-RPC message: not valid JSON")
	}
	if raw[0] != '{' {
		return nil, errorReply(nullID, codeInvalidRequest, "a JSON-RPC message is an object")
	}
	members := map[string]json.RawMessage{}
	bad := ""
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil, errorReply(nullID, codeParse, "parse JSON-RPC message: not valid JSON")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, errorReply(nullID, codeParse, "parse JSON-RPC message: not valid JSON")
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, errorReply(nullID, codeParse, "parse JSON-RPC message: not valid JSON")
		}
		for _, name := range reserved {
			switch {
			case key == name:
				if _, dup := members[name]; dup {
					bad = "member " + name + " appears twice"
				}
				members[name] = value
			case strings.EqualFold(key, name):
				bad = "member names are lower case: " + name
			}
		}
	}
	refuse := func(msg string) (*message, *reply) {
		id := nullID
		if v, ok := members["id"]; ok && validID(v) {
			id = v
		}
		return nil, errorReply(id, codeInvalidRequest, msg)
	}
	if bad != "" {
		return refuse(bad)
	}
	if string(bytes.TrimSpace(members["jsonrpc"])) != `"2.0"` {
		return refuse(`jsonrpc must be "2.0"`)
	}
	method := ""
	if v, ok := members["method"]; ok && !isNull(v) {
		if err := json.Unmarshal(v, &method); err != nil {
			return refuse("method is a string")
		}
	}
	if p, ok := members["params"]; ok {
		t := bytes.TrimSpace(p)
		if len(t) == 0 || (t[0] != '{' && t[0] != '[') {
			return refuse("params is an object or an array")
		}
	}
	msg := &message{method: method, params: members["params"], raw: raw}
	id, hasID := members["id"]
	if method == "" {
		_, hasResult := members["result"]
		_, hasError := members["error"]
		switch {
		case hasResult && hasError:
			return refuse("a response carries result or error, not both")
		case hasResult || hasError:
			msg.kind = kindResponse
			return msg, nil
		}
		return refuse("a request names a method")
	}
	if !hasID {
		msg.kind = kindNotification
		return msg, nil
	}
	if !validID(id) {
		return nil, errorReply(nullID, codeInvalidRequest, "id must be a string or a number")
	}
	msg.kind = kindRequest
	msg.id = bytes.TrimSpace(id)
	return msg, nil
}

func isNull(v json.RawMessage) bool { return string(bytes.TrimSpace(v)) == "null" }
