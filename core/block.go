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
	GetVersionMeta(id string) (BlockVersionMeta, error)
	// GetVersions returns the list of last entries
	GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error)
	// GetCurrentVersion returns the current(HEAD) version
	GetCurrentVersion() (BlockVersion, error)
	// GetCurrentVersionId returns the current(HEAD) version id of the block
	GetCurrentVersionId() (string, error)
	// NewBlock creates the new block but doesn't add it to the parent
	// make sure you add it later in AddVersions
	NewBlock(block model.Block) (Block, error)
	// AddVersion adds the new version of block's
	// if some model.Block fields are nil they will be taken from the current version.
	AddVersion(blockVersion *model.Block) (BlockVersion, error)
	// AddVersions adds the new version for the block itself and for any of it's dependents
	// if some model.Block fields are nil they will be taken from the current version.
	AddVersions(blockVersions []*model.Block) ([]BlockVersion, error)
	// EmptyVersion returns dumb BlockVersion, you can use it as a placeholder when no version yet created
	EmptyVersion() BlockVersion
	// GetNewVersionsOfBlocks sends the target block itself and dependent blocks' new versions to the chan
	SubscribeNewVersionsOfBlocks(sinceVersionId string, includeSinceVersion bool, blocks chan<- []BlockVersion) (cancelFunc func(), err error)
	// SubscribeMetaOfNewVersionsOfBlock subscribe for meta updates for the the block itself. You won't receive dependent(simple) block updates
	SubscribeMetaOfNewVersionsOfBlock(sinceVersionId string, includeSinceVersion bool, block chan<- BlockVersionMeta) (cancelFunc func(), err error)
	// SubscribeClientEvents provide a way to subscribe for the client-side events e.g. carriage position change
	SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func(), err error)
	// PublishClientEvent gives a way to push the new client-side event e.g. carriage position change
	// notice that you will also get this event in SubscribeForEvents
	PublishClientEvent(event proto.Message) error
}

type BlockVersion interface {
	VersionId() string
	Model() *model.Block
	User() string
	Date() *types.Timestamp
	// ExternalFields returns fields supposed to be viewable when block not opened
	ExternalFields() *types.Struct
	// DependentBlocks gives the initial version of dependent blocks
	// it can contain blocks in the not fully loaded state, e.g. images in the state of DOWNLOADING
	DependentBlocks() map[string]BlockVersion
}

type BlockVersionMeta interface {
	VersionId() string
	Model() *model.BlockMetaOnly
	User() string
	Date() *types.Timestamp
	// ExternalFields returns fields supposed to be viewable when block not opened
	ExternalFields() *types.Struct
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

func blockRestrictionsEmpty() model.BlockRestrictions {
	return model.BlockRestrictions{
		Read:   false,
		Edit:   false,
		Remove: false,
		Drag:   false,
		DropOn: false,
	}
}
