// Package compose bridges Heart's generated protobuf model to the portable
// bundle composer in github.com/anyproto/any-block.
package compose

import (
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
	Id       string
	SbType   model.SmartBlockType
	FileExt  string
	FileMime string
}

func BuildPlan(spaceId string, documents []DocMeta) (*Plan, error) {
	externalDocuments := make([]external.DocMeta, len(documents))
	for i, document := range documents {
		externalDocuments[i] = external.DocMeta{
			Id:       document.Id,
			SbType:   externalmodel.SmartBlockType(document.SbType),
			FileExt:  document.FileExt,
			FileMime: document.FileMime,
		}
	}
	return external.BuildPlan(spaceId, externalDocuments)
}

type Composer struct {
	inner *external.Composer
}

func NewComposer(options anyblockjson.Options, spaceName string) *Composer {
	return &Composer{inner: external.NewComposer(anyblockjson.ExternalOptions(options), spaceName)}
}

func (c *Composer) Observe(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase) (bool, []Issue) {
	externalSnapshot, err := anyblockjson.ToExternalSnapshot(snapshot)
	if err != nil {
		return false, []Issue{{Category: "protobuf_bridge", Detail: err.Error()}}
	}
	return c.inner.Observe(externalmodel.SmartBlockType(sbType), externalSnapshot)
}

func (c *Composer) ObserveWritten(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, document []byte, path string) error {
	externalSnapshot, err := anyblockjson.ToExternalSnapshot(snapshot)
	if err != nil {
		return err
	}
	return c.inner.ObserveWritten(externalmodel.SmartBlockType(sbType), externalSnapshot, document, path)
}

func (c *Composer) ObserveFileBlob(objectId, path string) {
	c.inner.ObserveFileBlob(objectId, path)
}

func (c *Composer) Finish() (index, properties []byte, stats Stats, err error) {
	return c.inner.Finish()
}
