package schema

import (
	"fmt"

	ipld "github.com/ipfs/go-ipld-format"

	"github.com/anytypeio/go-anytype-library/pb/storage"
)

// ErrFileValidationFailed indicates dag schema validation failed
var ErrFileValidationFailed = fmt.Errorf("file failed schema validation")

// ErrEmptySchema indicates a schema is empty
var ErrEmptySchema = fmt.Errorf("schema does not create any files")

// ErrLinkOrderNotSolvable
var ErrLinkOrderNotSolvable = fmt.Errorf("link order is not solvable")

// ErrSchemaInvalidMill indicates a schema has an invalid mill entry
var ErrSchemaInvalidMill = fmt.Errorf("schema contains an invalid mill")

// ErrMissingJsonSchema indicates json schema is missing
var ErrMissingJsonSchema = fmt.Errorf("json mill requires a json schema")

// ErrBadJsonSchema indicates json schema is invalid
var ErrBadJsonSchema = fmt.Errorf("json schema is not valid")

// FileTag indicates the link should "use" the input file as source
const FileTag = ":file"

// SingleFileTag is a magic key indicating that a directory is actually a single file
const SingleFileTag = ":single"

// ValidateMill is false if mill is not one of the built in tags
func ValidateMill(mill string) bool {
	switch mill {
	case
		"/schema",
		"/blob",
		"/image/resize",
		"/image/exif",
		"/json":
		return true
	}
	return false
}

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

// Steps returns link steps in the order they should be processed
func Steps(links map[string]*storage.Link) ([]storage.Step, error) {
	var steps []storage.Step
	run := links
	i := 0
	for {
		if i > len(links) {
			return nil, ErrLinkOrderNotSolvable
		}
		next := orderLinks(run, &steps)
		if len(next) == 0 {
			break
		}
		run = next
		i++
	}
	return steps, nil
}

// orderLinks attempts to place all links in steps, returning any unused
// whose source is not yet in steps
func orderLinks(links map[string]*storage.Link, steps *[]storage.Step) map[string]*storage.Link {
	unused := make(map[string]*storage.Link)
	for name, link := range links {
		if link.Use == FileTag {
			*steps = append([]storage.Step{{Name: name, Link: link}}, *steps...)
		} else {
			useAt := -1
			for i, s := range *steps {
				if link.Use == s.Name {
					useAt = i
					break
				}
			}
			if useAt >= 0 {
				*steps = append(*steps, storage.Step{Name: name, Link: link})
			} else {
				unused[name] = link
			}
		}
	}
	return unused
}
