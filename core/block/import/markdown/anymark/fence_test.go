package anymark

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// GO-7515: html-to-markdown's default after hook right-trims every line of the
// finished document and collapses blank runs, with no idea where code fences
// are. Inside a fence that whitespace is content.
func TestTrimMarkdownOutsideFences(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trailing spaces inside a fence are kept",
			in:   "```\na  \n```",
			want: "```\na  \n```",
		},
		{
			name: "a line of only spaces inside a fence is kept",
			in:   "```\n   \n```",
			want: "```\n   \n```",
		},
		{
			name: "blank runs inside a fence are kept",
			in:   "```\na\n\n\n\nb\n```",
			want: "```\na\n\n\n\nb\n```",
		},
		{
			name: "trailing spaces outside a fence are trimmed",
			in:   "para   \nmore  ",
			want: "para\nmore",
		},
		{
			name: "blank runs outside a fence collapse to one",
			in:   "a\n\n\n\nb",
			want: "a\n\nb",
		},
		{
			name: "text around a fence is trimmed while the fence is not",
			in:   "para   \n\n```\nx  \n```\n\nafter   ",
			want: "para\n\n```\nx  \n```\n\nafter",
		},
		{
			name: "tilde fences are honoured too",
			in:   "~~~\na  \n~~~",
			want: "~~~\na  \n~~~",
		},
		{
			name: "a longer closing run closes the fence",
			in:   "```\na  \n`````\nb   ",
			want: "```\na  \n`````\nb",
		},
		{
			name: "a shorter run inside a fence does not close it",
			in:   "````\na  \n```\nb  \n````\nc   ",
			want: "````\na  \n```\nb  \n````\nc",
		},
		{
			name: "an inline code span is not a fence opener",
			in:   "``a``b``   \nnext   ",
			want: "``a``b``\nnext",
		},
		{
			name: "a backtick run whose info string holds a backtick is not a fence",
			in:   "```a``b```   \nnext   ",
			want: "```a``b```\nnext",
		},
		{
			// Four spaces makes it indented code, not a fence, so the lines after
			// it keep being treated as ordinary text and are trimmed.
			name: "a fence indented more than three spaces is not a fence",
			in:   "x\n    ```   \n    a  ",
			want: "x\n    ```\n    a",
		},
		{
			name: "a fence indented up to three spaces is a fence",
			in:   "x\n   ```\n   a  \n   ```",
			want: "x\n   ```\n   a  \n   ```",
		},
		{
			name: "document edges are trimmed",
			in:   "\n\n  para  \n\n",
			want: "para",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			// The document-level TrimSpace still applies at the very end, exactly
			// as it does in the hook this replaces.
			name: "an unterminated fence keeps its interior verbatim",
			in:   "```\na  \nb  \nc",
			want: "```\na  \nb  \nc",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, trimMarkdownOutsideFences(tc.in))
		})
	}
}

func TestParseFenceOpen(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         string
		wantChar   byte
		wantLength int
		wantOk     bool
	}{
		{name: "three backticks", in: "```", wantChar: '`', wantLength: 3, wantOk: true},
		{name: "three backticks with an info string", in: "```go", wantChar: '`', wantLength: 3, wantOk: true},
		{name: "five backticks", in: "`````", wantChar: '`', wantLength: 5, wantOk: true},
		{name: "three tildes", in: "~~~", wantChar: '~', wantLength: 3, wantOk: true},
		{name: "tilde info string may hold a backtick", in: "~~~a`b", wantChar: '~', wantLength: 3, wantOk: true},
		{name: "indented up to three spaces", in: "   ```", wantChar: '`', wantLength: 3, wantOk: true},
		{name: "two backticks is not a fence", in: "``", wantOk: false},
		{name: "backtick info string holding a backtick is not a fence", in: "```a``b```", wantOk: false},
		{name: "indented four spaces is not a fence", in: "    ```", wantOk: false},
		{name: "ordinary text", in: "hello", wantOk: false},
		{name: "empty", in: "", wantOk: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			char, length, ok := parseFenceOpen(tc.in)
			assert.Equal(t, tc.wantOk, ok)
			if tc.wantOk {
				assert.Equal(t, tc.wantChar, char)
				assert.Equal(t, tc.wantLength, length)
			}
		})
	}
}

func TestIsFenceClose(t *testing.T) {
	for _, tc := range []struct {
		name      string
		line      string
		fenceChar byte
		fenceLen  int
		want      bool
	}{
		{name: "exact match", line: "```", fenceChar: '`', fenceLen: 3, want: true},
		{name: "longer run closes", line: "`````", fenceChar: '`', fenceLen: 3, want: true},
		{name: "shorter run does not close", line: "```", fenceChar: '`', fenceLen: 4, want: false},
		{name: "trailing whitespace is allowed", line: "```   ", fenceChar: '`', fenceLen: 3, want: true},
		{name: "trailing content is not allowed", line: "``` x", fenceChar: '`', fenceLen: 3, want: false},
		{name: "a different fence char does not close", line: "~~~", fenceChar: '`', fenceLen: 3, want: false},
		{name: "indented up to three spaces closes", line: "   ```", fenceChar: '`', fenceLen: 3, want: true},
		{name: "indented four spaces does not close", line: "    ```", fenceChar: '`', fenceLen: 3, want: false},
		{name: "code content does not close", line: "a  ", fenceChar: '`', fenceLen: 3, want: false},
		{name: "empty line does not close", line: "", fenceChar: '`', fenceLen: 3, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isFenceClose(tc.line, tc.fenceChar, tc.fenceLen))
		})
	}
}
