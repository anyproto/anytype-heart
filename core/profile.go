package core

import (
	"fmt"
	"regexp"

	"github.com/gogo/protobuf/types"
	mh "github.com/multiformats/go-multihash"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/anytypeio/go-anytype-library/vclock"
)

func (a *Anytype) AccountSetNameAndAvatar(name string, avatarFilePath, color string) error {
	block, err := a.GetBlock(a.predefinedBlockIds.Profile)
	if err != nil {
		return err
	}

	if snapshot, _ := block.GetLastSnapshot(); snapshot == nil {
		// snapshot not yet created
		log.Debugf("add predefined profile block snapshot")
		_, err = block.PushSnapshot(vclock.New(), &SmartBlockMeta{
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					"name":  structs.String(name),
					"image": structs.String(avatarFilePath),
					"color": structs.String(color),
				},
			},
		}, []*model.Block{
			// todo: add title and avatar blocks
		})

		if err != nil {
			return err
		}
	}

	return nil
}

var hexColorRegexp = regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)
var invalidHexColor = fmt.Errorf("HEX color has invalid format")

func (a *Anytype) AccountSetAvatarColor(hex string) (err error) {
	return fmt.Errorf("not implemented")
}

func (a *Anytype) AccountSetAvatar(localPath string) (hash mh.Multihash, err error) {
	return nil, fmt.Errorf("not implemented")
}

/*func (a *Anytype) AccountRequestStoredContact(ctx context.Context, accountId string) (contact *tpb.Contact, err error) {
	return nil, fmt.Errorf("not implemented")
}*/
