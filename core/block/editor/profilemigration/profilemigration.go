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
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, InternalKeyOldProfileData)
	if err != nil {
		return nil, err
	}
	newState := state.NewDocWithUniqueKey(st.RootId(), blocksMap, uk).(*state.State)
	newState.AddRelationLinks(st.GetRelationLinks()...)
	newStateDetails := pbtypes.CopyStruct(st.Details(), true)
	newName := pbtypes.GetString(newStateDetails, bundle.RelationKeyName.String()) + " [migrated]"
	newStateDetails.Fields[bundle.RelationKeyName.String()] = pbtypes.String(newName)
	newStateDetails.Fields[bundle.RelationKeyIsHidden.String()] = pbtypes.Bool(false)
	newState.SetDetails(newStateDetails)
	// remove the identity block
	newState.Unlink(identityBlockId)
	newState.CleanupBlock(identityBlockId)
	newState.SetObjectTypeKey(bundle.TypeKeyPage)

	// now cleanup the original state
	rootBlock := st.Get(st.RootId())
	rootBlock.Model().ChildrenIds = slices.DeleteFunc(rootBlock.Model().ChildrenIds, func(s string) bool {
		return !slices.Contains(whitelistBlocks, s)
	})

	whitelistDetailKeys := []string{
		"iconEmoji",
		"name",
		"isHidden",
		"featuredRelations",
		"layout",
		"layoutAlign",
		"iconImage",
		"iconOption",
	}
	var keysToRemove []string
	for k := range st.Details().GetFields() {
		if !slices.Contains(whitelistDetailKeys, k) {
			keysToRemove = append(keysToRemove, k)
		}
	}
	// cleanup custom details from old state
	st.RemoveDetail(keysToRemove...)
	st.RemoveRelation(keysToRemove...)
	return newState, nil
}

type IsEmpty interface {
	IsEmpty() bool
}
