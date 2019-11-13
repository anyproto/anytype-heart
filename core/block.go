package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	tcore "github.com/textileio/go-textile/core"
)

type Block interface {
	GetId() string
	GetVersion(id string) (BlockVersion, error)
	// GetVersions returns the list of last entries
	GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error)
	// GetCurrentVersionId returns the current(HEAD) version id of the block
	GetCurrentVersion() (BlockVersion, error)
	// AddVersion adds the new version of block's
	// if arg is nil it will be taken from the last version
	AddVersion(dependentBlocks map[string]BlockVersion, fields *types.Struct, children []string, content model.IsBlockContent) error
	// SubscribeForEvents provide a way to subscribe for the block and its children events
	SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func())
	// PublishClientEvent gives a way to push the new client-side event e.g. carriage position change
	// notice that you will also get this event in SubscribeForEvents
	PublishClientEvent(event proto.Message)
}

type BlockVersion interface {
	GetBlockId() string
	GetVersionId() string
	GetUser() string
	GetDate() *types.Timestamp
	// GetChildrenIds returns IDs of children blocks
	GetChildrenIds() []string
	// GetPermissions returns permissions
	GetPermissions() *model.BlockPermissions
	// GetExternalFields returns fields supposed to be viewable when block not opened
	GetExternalFields() *types.Struct
	// GetFields returns all block fields
	GetFields() *types.Struct
	// GetContent returns the content interface
	GetContent() model.IsBlockContent
	// GetDependentBlocks gives the initial version of dependent blocks
	// it can contain blocks in the not fully loaded state, e.g. images in the state of DOWNLOADING
	GetDependentBlocks() map[string]BlockVersion
	// GetNewVersionsOfBlocks sends the target block itself and dependent blocks' new versions to the chan
	// it can send the same block version in case the status changes  (e.g. DOWNLOADING -> PREVIEW for an image block)
	GetNewVersionsOfBlocks(blocks chan<- []BlockVersion) (cancelFunc func())
}

var ErrorNotSmartBlock = fmt.Errorf("can't retrieve thread for not smart block")

func (anytype *Anytype) getThreadForBlock(b *model.Block) (*tcore.Thread, error) {
	switch b.Content.(type) {
	case *model.BlockContentOfPage, *model.BlockContentOfDashboard:
		return anytype.Textile.Node().Thread(b.Id), nil
	default:
		return nil, ErrorNotSmartBlock
	}
}
