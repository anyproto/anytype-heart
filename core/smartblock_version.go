package core

import (
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/types"
	mh "github.com/multiformats/go-multihash"
)

type SmartBlockVersion struct {
	model     *storage.BlockWithMeta
	versionId string
	user      string
	date      *types.Timestamp
	node      *Anytype
}

func (version *SmartBlockVersion) Model() *model.Block {
	return version.model.Block
}

func (version *SmartBlockVersion) VersionId() string {
	return version.versionId
}

func (version *SmartBlockVersion) User() string {
	return version.user
}

func (version *SmartBlockVersion) Date() *types.Timestamp {
	return version.date
}

func (version *SmartBlockVersion) GetContent() model.IsBlockContent {
	return version.model.Block.Content
}

func (version *SmartBlockVersion) DependentBlocks() map[string]BlockVersion {
	var allChildren = version.Model().ChildrenIds
	var allChildrenMap = make(map[string]struct{}, 0)
	var m = make(map[string]BlockVersion, len(version.model.BlockById))
	for blockId, block := range version.model.BlockById {
		switch block.Content.(type) {
		case *model.BlockContentOfDashboard, *model.BlockContentOfPage:
			// not supported

		default:
			m[blockId] = &SimpleBlockVersion{
				model:                   block,
				parentSmartBlockVersion: version,
			}

			for _, child := range block.ChildrenIds {
				if _, exists := allChildrenMap[child]; !exists {
					allChildren = append(allChildren, child)
					allChildrenMap[child] = struct{}{}
				}
			}
		}
	}

	// inject smart blocks children
	for _, child := range version.model.Block.ChildrenIds {
		if _, err := mh.FromB58String(child); err != nil {
			if _, exists := m[child]; !exists {
				log.Errorf("DependentBlocks: children simple block '%s' not presented in the stored dependent blocks of smart block '%s'", child, version.model.Block.Id)
			}

			continue
		}

		smartBlock, err := version.node.GetBlock(child)
		if err != nil {
			m[child] = version.node.smartBlockVersionWithFullRestrictions(child)
		} else {

			smartBlockVersion, err := smartBlock.GetCurrentVersion()
			if err != nil {
				m[child] = smartBlock.EmptyVersion()
				continue
			}

			m[child] = smartBlockVersion
		}
	}
	return m
}

func (version *SmartBlockVersion) ExternalFields() *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		"name": version.Model().Fields.Fields["name"],
		"icon": version.Model().Fields.Fields["icon"],
	}}
}

type rowHash struct {
	Hash string
}

// addMissingFiles ensure that all fileIndex exist in this version added to the files database
func (version *SmartBlockVersion) addMissingFiles() error {
	if len(version.model.FileByHash) == 0 {
		return nil
	}

	hashes := make([]string, 0, len(version.model.FileByHash))
	for hash, _ := range version.model.FileByHash {
		hashes = append(hashes, hash)
	}
	hashesString := "\"" + strings.Join(hashes, "\",\"") + "\""

	rows, err := version.node.Textile.Node().Datastore().Files().PrepareAndExecuteQuery("select hash from files where hash in(" + hashesString + ")")
	if err != nil {
		return err
	}

	filesExists := make(map[string]struct{})
	var row rowHash
	for rows.Next() {
		err = rows.Scan(&row)
		if err != nil {
			return err
		}
		filesExists[row.Hash] = struct{}{}
	}

	for hash, file := range version.model.FileByHash {
		if _, exists := filesExists[hash]; exists {
			continue
		}
		err = version.node.Textile.Node().Datastore().Files().Add(util.CastFileIndexToTextile(file))
		if err != nil {
			log.Errorf("smartblock: add a missing file to db got error: %s", err.Error())
		}
	}

	return nil
}
