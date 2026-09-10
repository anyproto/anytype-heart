// Package filterstring preserves Heart's former import path while the parser
// is owned and implemented by github.com/anyproto/any-block.
package filterstring

import external "github.com/anyproto/any-block/codec/anyblockjson/filterstring"

const EBNF = external.EBNF

type (
	Options = external.Options
	Error   = external.Error
)

var (
	Examples  = external.Examples
	Parse     = external.Parse
	ParseDate = external.ParseDate
)
