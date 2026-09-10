// Package compose bridges Heart's generated protobuf model to the portable
// bundle composer in github.com/anyproto/any-block.
package compose

import (
	external "github.com/anyproto/any-block/bundle"
	externalmodel "github.com/anyproto/any-block/format/v1/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const IssueOptionDescriptionOmitted = external.IssueOptionDescriptionOmitted
const IssueOptionContentOmitted = external.IssueOptionContentOmitted

type (
	Issue = external.Issue
	Stats = external.Stats
	Plan  = external.Plan
)

type DocMeta struct {
	Id     string
	SbType model.SmartBlockType
	// Key is the snapshot's own Key — for a type document the internal key
	// its envelope id `type-<Key>` is derived from (any-block SPEC §9). Left
	// empty, every type document reverts to its raw store id.
	Key      string
	FileExt  string
	FileMime string
}

func BuildPlan(options anyblockjson.Options, documents []DocMeta) (*Plan, error) {
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
	return external.BuildPlan(anyblockjson.ExternalOptions(options), externalDocuments)
}

type Composer struct {
	inner *external.Composer
}

// NewComposer returns an error for Options a bundle refuses — currently
// NoDerivedTypeIds, which is a single document's export mode (any-block SPEC
// §9): declining the type fold files a type document under its store id, and a
// bundle reaches a type document only by its derived id.
func NewComposer(options anyblockjson.Options, spaceName string) (*Composer, error) {
	inner, err := external.NewComposer(anyblockjson.ExternalOptions(options), spaceName)
	if err != nil {
		return nil, err
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

// UsedPropertyKeysFromBytes re-exports the composer's own used-key census, so
// a tool that scans a written bundle asks the same question the composer asked
// while writing it rather than re-deriving which slots name a property.
func UsedPropertyKeysFromBytes(document []byte) (map[string]bool, error) {
	return external.UsedPropertyKeysFromBytes(document)
}
