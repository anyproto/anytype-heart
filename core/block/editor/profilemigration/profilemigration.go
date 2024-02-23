package profilemigration

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const InternalKeyOldProfileData = "oldprofile"

var ErrNoCustomStateFound = fmt.Errorf("no custom state found")

// ExtractCustomState extract user-added state to the separate state and removes all the custom blocks/details from the original one
func ExtractCustomState(st *state.State) (userState *state.State, err error) {
	identityBlockId := "identity"
	// we leave identity and other blocks in the original object to avoid them being re-added by old clients
	whitelistBlocks := []string{state.HeaderLayoutID, state.DescriptionBlockID, state.FeaturedRelationsID, state.TitleBlockID, identityBlockId}
	hasCustomState := false
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if slices.Contains(whitelistBlocks, b.Model().Id) {
			return true
		}
		if textBlock, ok := b.(text.Block); ok {
			// custom one for text block
			if strings.TrimSpace(textBlock.GetText()) != "" {
				hasCustomState = true
				return false
			}
		} else if emptyChecker, ok := b.(IsEmpty); ok && !emptyChecker.IsEmpty() {
			hasCustomState = true
			return false
		}
		return true
	})
	if !hasCustomState {
		return nil, ErrNoCustomStateFound
	}
	blocksMap := map[string]simple.Block{}

	st.Iterate(func(b simple.Block) (isContinue bool) {
		blocksMap[b.Model().Id] = b.Copy()
		return true
	})

	uk, err := domain.NewUniqueKey(smartblock2.SmartBlockTypePage, InternalKeyOldProfileData)
	if err != nil {
		return nil, err
	}
	newState := state.NewDocWithUniqueKey(st.RootId(), blocksMap, uk).(*state.State)
	newState.SetDetails(pbtypes.CopyStruct(st.CombinedDetails()))
	newName := pbtypes.GetString(newState.Details(), bundle.RelationKeyName.String()) + " [migrated]"
	newState.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(newName))
	newState.SetDetail(bundle.RelationKeyIsHidden.String(), pbtypes.Bool(false))
	newState.SetDetail(bundle.RelationKeyIsReadonly.String(), pbtypes.Bool(false))
	newState.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_profile)))
	newState.CleanupBlock(identityBlockId)

	rootBlock := st.Pick(st.RootId())
	slices.DeleteFunc(rootBlock.Model().ChildrenIds, func(s string) bool {
		return !slices.Contains(whitelistBlocks, s)
	})

	var whitelistDetailKeys = []string{
		"iconEmoji",
		"name",
		"isHidden",
		"featuredRelations",
		"layout",
		"layoutAlign",
		"iconImage",
		"iconOption",
	}
	keysToRemove := []string{}
	for k := range st.Details().GetFields() {
		if !slices.Contains(whitelistDetailKeys, k) {
			keysToRemove = append(keysToRemove, k)
		}
	}

	st.RemoveDetail(keysToRemove...)
	st.RemoveRelation(keysToRemove...)
	return newState, nil
}

type IsEmpty interface {
	IsEmpty() bool
}
