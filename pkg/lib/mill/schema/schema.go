package schema

import (
	"fmt"

	ipld "github.com/ipfs/go-ipld-format"
)

// ErrEmptySchema indicates a schema is empty
var ErrEmptySchema = fmt.Errorf("schema does not create any files")

// ErrLinkOrderNotSolvable
var ErrLinkOrderNotSolvable = fmt.Errorf("link order is not solvable")

// FileTag indicates the link should "use" the input file as source
const FileTag = ":file"

// SingleFileTag is a magic key indicating that a directory is actually a single file
const SingleFileTag = ":single"

// LinkByName finds a link w/ one of the given names in the provided list
func LinkByName(links []*ipld.Link, names []string) *ipld.Link {
	for _, l := range links {
		for _, n := range names {
			if l.Name == n {
				return l
			}
		}
	}
	return nil
}
