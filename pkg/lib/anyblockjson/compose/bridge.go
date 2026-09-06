// Package compose bridges Heart's generated protobuf model to the portable
// bundle composer in github.com/anyproto/any-block.
package compose

import (
	"fmt"
	external "github.com/anyproto/any-block/bundle"
	externalmodel "github.com/anyproto/any-block/format/v1/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type (
	Issue = external.Issue
	Stats = external.Stats
	Plan  = external.Plan
)

type DocMeta struct {
	Id     string
	SbType model.SmartBlockType
	// Key is the document's own internal key, for the kinds that have one
	// (types, properties). The path plan names a file by the document's
	// ENVELOPE id, and a type document's envelope id is derived from its own
	// key rather than from a resolver (SPEC §9) — so a plan built without it
	// would name files that the documents written into them disagree with.
	Key      string
	FileExt  string
	FileMime string
}

// BuildPlan names every document's file. opts must be the SAME options the
// documents are marshaled with: the plan and the envelope both go through
// FoldDocumentId, and options that disagree (a fold on one side, off on the
// other) put a document whose envelope says one id into a file named for
// another.
func BuildPlan(opts anyblockjson.Options, documents []DocMeta) (*Plan, error) {
	externalDocuments := make([]external.DocMeta, len(documents))
	for i, document := range documents {
		externalDocuments[i] = external.DocMeta{
			Id:       document.Id,
			SbType:   externalmodel.SmartBlockType(document.SbType),
			Key:      document.Key,
			FileExt:  document.FileExt,
			FileMime: document.FileMime,
		}
	}
	return external.BuildPlan(anyblockjson.ExternalOptions(opts), externalDocuments)
}

type Composer struct {
	inner *external.Composer
}

// NewComposer refuses Options a bundle cannot be composed from — today, the
// document-only NoDerivedTypeIds mode, which changes what a type document is
// ADDRESSED by and so removes a bundle's only road to a type. It refuses at
// construction rather than at Finish, when every document has already been
// emitted and the news is useless.
func NewComposer(options anyblockjson.Options, spaceName string) (*Composer, error) {
	inner, err := external.NewComposer(anyblockjson.ExternalOptions(options), spaceName)
	if err != nil {
		return nil, fmt.Errorf("new composer: %w", err)
	}
	return &Composer{inner: inner}, nil
}

func (c *Composer) Observe(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase) (bool, []Issue) {
	externalSnapshot, err := anyblockjson.ToExternalSnapshot(snapshot)
	if err != nil {
		return false, []Issue{{Category: "protobuf_bridge", Detail: err.Error()}}
	}
	return c.inner.Observe(externalmodel.SmartBlockType(sbType), externalSnapshot)
}

func (c *Composer) ObserveWritten(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, document []byte) error {
	externalSnapshot, err := anyblockjson.ToExternalSnapshot(snapshot)
	if err != nil {
		return err
	}
	return c.inner.ObserveWritten(externalmodel.SmartBlockType(sbType), externalSnapshot, document)
}

func (c *Composer) ObserveFileBlob(objectId, path string) {
	c.inner.ObserveFileBlob(objectId, path)
}

func (c *Composer) Finish() (index, properties []byte, stats Stats, err error) {
	return c.inner.Finish()
}
