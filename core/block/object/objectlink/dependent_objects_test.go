package objectlink

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fakeConverter struct {
}

func (f *fakeConverter) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	return fakeDerivedID(key.String()), nil
}

func (f *fakeConverter) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	return fakeDerivedID(key.String()), nil
}

func (f *fakeConverter) Id() string {
	return ""
}

func setupFetcher(t *testing.T, links pbtypes.RelationLinks) relationutils.RelationFormatFetcher {
	fetcher := mock_relationutils.NewMockRelationFormatFetcher(t)
	fetcher.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ string, key domain.RelationKey) (model.RelationFormat, error) {
		rel, err := bundle.GetRelation(key)
		if err == nil {
			return rel.Format, nil
		}
		link := links.Get(key.String())
		if link != nil {
			return link.Format, nil
		}
		return 0, err
	}).Maybe()
	return fetcher
}

func fakeDerivedID(key string) string {
	return fmt.Sprintf("derivedFrom(%s)", key)
}

type fakeSpaceIdResolver struct {
	idsToSpaceIds map[string]string
}

func (r *fakeSpaceIdResolver) ResolveSpaceID(id string) (string, error) {
	spaceId, found := r.idsToSpaceIds[id]
	if !found {
		return "", fmt.Errorf("not found")
	}
	return spaceId, nil
}

func TestState_DepSmartIdsLinks(t *testing.T) {
	// given
	stateWithLinks := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock2", "childBlock3"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   8,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "objectID",
						},
						{
							Range: &model.Range{
								From: 9,
								To:   19,
							},
							Type:  model.BlockContentTextMark_Mention,
							Param: "objectID2",
						},
					},
				}},
			}}),
		"childBlock2": simple.New(&model.Block{Id: "childBlock2",
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					TargetObjectId: "objectID3",
				},
			}}),
		"childBlock3": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "objectID4",
				},
			}}),
	}).(*state.State)
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, nil)

	t.Run("block option is turned on: get ids from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true})
		assert.Len(t, objectIDs, 4)
	})

	t.Run("all options are turned off", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{})
		assert.Len(t, objectIDs, 0)
	})
}

func TestState_DepSmartIdsLinksAndRelations(t *testing.T) {
	// given
	dateObject1 := dateutil.NewDateObject(time.Now(), true)
	dateObject2 := dateutil.NewDateObject(time.Now(), false)
	stateWithLinks := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock2", "childBlock3", "dataview", "image", "song", "date1", "date2"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   8,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "objectID",
						},
						{
							Range: &model.Range{
								From: 9,
								To:   19,
							},
							Type:  model.BlockContentTextMark_Mention,
							Param: "objectID2",
						},
					},
				}},
			}}),
		"childBlock2": simple.New(&model.Block{Id: "childBlock2",
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					TargetObjectId: "objectID3",
				},
			}}),
		"childBlock3": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "objectID4",
				},
			}}),
		"dataview": simple.New(&model.Block{Id: "dataview",
			Content: &model.BlockContentOfDataview{
				Dataview: &model.BlockContentDataview{
					Views: []*model.BlockContentDataviewView{{
						Id:                  "Today's tasks",
						DefaultObjectTypeId: "task",
						DefaultTemplateId:   "Task with a picture",
					}},
					TargetObjectId: "taskTracker",
				},
			}}),
		"image": simple.New(&model.Block{Id: "image",
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					TargetObjectId: "image with cute kitten",
					Type:           model.BlockContentFile_Image,
				},
			}}),
		"song": simple.New(&model.Block{Id: "song",
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					TargetObjectId: "Let it be",
					Type:           model.BlockContentFile_Audio,
				},
			}}),
		"date1": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: dateObject1.Id(),
				},
			}}),
		"date2": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: dateObject2.Id(),
				},
			}}),
	}).(*state.State)
	converter := &fakeConverter{}

	relations := []*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_file,
		},
		{
			Key:    "relation2",
			Format: model.RelationFormat_tag,
		},
		{
			Key:    "relation3",
			Format: model.RelationFormat_status,
		},
		{
			Key:    "relation4",
			Format: model.RelationFormat_object,
		},
	}
	stateWithLinks.AddRelationLinks(relations...)
	stateWithLinks.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"relation1": domain.String("image_with_cute_kitten"),
		"relation2": domain.String("Important"),
		"relation3": domain.String("TODO"),
		"relation4": domain.String("Project"),
	}))
	fetcher := setupFetcher(t, relations)

	t.Run("blocks option is turned on: get ids from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true})
		assert.Len(t, objectIDs, 11)
	})

	t.Run("dataview only target option is turned on: get only target from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true, DataviewBlockOnlyTarget: true})
		assert.Len(t, objectIDs, 9)
	})

	t.Run("no images option is turned on: get ids from blocks except images", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true, NoImages: true})
		assert.Len(t, objectIDs, 10)
	})

	t.Run("blocks option and relations options are turned on: get ids from blocks and relations", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true, Relations: true})
		assert.Len(t, objectIDs, 15) // 11 links + 4 relations
	})

	t.Run("save backlinks", func(t *testing.T) {
		st := stateWithLinks.Copy()
		st.SetDetail(bundle.RelationKeyBacklinks, domain.StringList([]string{"link1"}))
		st.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyBacklinks.String(),
			Format: model.RelationFormat_object,
		})
		objectIDs := DependentObjectIDs(st, converter, fetcher, Flags{Details: true})
		assert.Len(t, objectIDs, 1)
		assert.Contains(t, objectIDs, "link1")
	})
	t.Run("skip backlinks", func(t *testing.T) {
		st := stateWithLinks.Copy()
		st.SetDetail(bundle.RelationKeyBacklinks, domain.StringList([]string{"link1"}))
		st.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyBacklinks.String(),
			Format: model.RelationFormat_object,
		})
		objectIDs := DependentObjectIDs(st, converter, fetcher, Flags{Details: true, NoBackLinks: true})
		assert.Len(t, objectIDs, 0)
	})
}

func buildStateWithLinks() *state.State {
	stateWithLinks := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Id:          "root",
			ChildrenIds: []string{"childBlock", "childBlock2", "childBlock3"},
		}),
		"childBlock": simple.New(&model.Block{Id: "childBlock",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   8,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "objectID",
						},
						{
							Range: &model.Range{
								From: 9,
								To:   19,
							},
							Type:  model.BlockContentTextMark_Mention,
							Param: "objectID2",
						},
					},
				}},
			}}),
		"childBlock2": simple.New(&model.Block{Id: "childBlock2",
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					TargetObjectId: "objectID3",
				},
			}}),
		"childBlock3": simple.New(&model.Block{Id: "childBlock3",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: "objectID4",
				},
			}}),
	}).(*state.State)

	relations := []*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_file,
		},
		{
			Key:    "relation2",
			Format: model.RelationFormat_tag,
		},
		{
			Key:    "relation3",
			Format: model.RelationFormat_status,
		},
		{
			Key:    "relation4",
			Format: model.RelationFormat_object,
		},
		{
			Key:    "relation5",
			Format: model.RelationFormat_date,
		},
	}
	stateWithLinks.AddRelationLinks(relations...)
	stateWithLinks.SetDetail("relation1", domain.StringList([]string{"file"}))
	stateWithLinks.SetDetail("relation2", domain.StringList([]string{"option1"}))
	stateWithLinks.SetDetail("relation3", domain.StringList([]string{"option2"}))
	stateWithLinks.SetDetail("relation4", domain.StringList([]string{"option3"}))
	stateWithLinks.SetDetail("relation5", domain.Int64(time.Now().Unix()))

	return stateWithLinks
}

func TestState_DepSmartIdsLinksDetailsAndRelations(t *testing.T) {
	// given
	stateWithLinks := buildStateWithLinks()
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, stateWithLinks.PickRelationLinks())

	t.Run("blocks option is turned on: get ids from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true})
		assert.Len(t, objectIDs, 4) // links
	})
	t.Run("blocks option and relations option are turned on: get ids from blocks and relations", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true, Relations: true})
		assert.Len(t, objectIDs, 9) // 4 links + 5 relations
	})
	t.Run("blocks, relations and details option are turned on: get ids from blocks, relations and details", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Blocks: true, Relations: true, Details: true})
		assert.Len(t, objectIDs, 14) // 4 links + 5 relations + 3 options + 1 fileID + 1 date
	})
}

func TestState_DepSmartIdsLinksCreatorModifierWorkspace(t *testing.T) {
	// given
	stateWithLinks := state.NewDoc("root", nil).(*state.State)
	relations := []*model.RelationLink{
		{
			Key:    "relation1",
			Format: model.RelationFormat_date,
		},
		{
			Key:    bundle.RelationKeyCreatedDate.String(),
			Format: model.RelationFormat_date,
		},
		{
			Key:    bundle.RelationKeyCreator.String(),
			Format: model.RelationFormat_object,
		},
		{
			Key:    bundle.RelationKeyLastModifiedBy.String(),
			Format: model.RelationFormat_object,
		},
	}
	stateWithLinks.AddRelationLinks(relations...)
	stateWithLinks.SetDetail("relation1", domain.Int64(time.Now().Unix()))
	stateWithLinks.SetDetail(bundle.RelationKeyCreatedDate, domain.Int64(time.Now().Unix()))
	stateWithLinks.SetDetail(bundle.RelationKeyCreator, domain.String("creator"))
	stateWithLinks.SetDetail(bundle.RelationKeyLastModifiedBy, domain.String("lastModifiedBy"))
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, relations)

	t.Run("details option is turned on: get ids only from details", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Details: true, CreatorModifierWorkspace: true})
		assert.Len(t, objectIDs, 3) // creator + lastModifiedBy + 1 date
	})

	t.Run("details and relations options are turned on: get ids from details and relations", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Details: true, Relations: true, CreatorModifierWorkspace: true})
		assert.Len(t, objectIDs, 7) // 4 relations + creator + lastModifiedBy + 1 date
	})
}

func TestState_DepSmartIdsObjectTypes(t *testing.T) {
	// given
	stateWithLinks := state.NewDoc("root", nil).(*state.State)
	stateWithLinks.SetObjectTypeKey(bundle.TypeKeyPage)
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, stateWithLinks.PickRelationLinks())

	t.Run("all options are turned off", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{})
		assert.Len(t, objectIDs, 0)
	})
	t.Run("objTypes option is turned on, get only object types id", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, fetcher, Flags{Types: true})
		assert.Equal(t, []string{
			fakeDerivedID(bundle.TypeKeyPage.String()),
		}, objectIDs)
	})
}

func TestDependentObjectLinks_AttributesBlockAndRelation(t *testing.T) {
	// given a state with one link block and one object relation
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"link1"}}),
		"link1": simple.New(&model.Block{
			Id: "link1",
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{TargetBlockId: "tgtBlock"},
			},
		}),
	}).(*state.State)
	st.AddRelationLinks(&model.RelationLink{
		Key:    "assignee",
		Format: model.RelationFormat_object,
	})
	st.SetDetail("assignee", domain.StringList([]string{"tgtRel"}))

	converter := &fakeConverter{}
	fetcher := setupFetcher(t, st.PickRelationLinks())

	// when
	links := DependentObjectLinks(st, converter, fetcher, Flags{
		Blocks:  true,
		Details: true,
	})

	// then
	require.Len(t, links, 2)
	byTarget := map[string]OutgoingLink{}
	for _, l := range links {
		byTarget[l.TargetID] = l
	}
	assert.Equal(t, "link1", byTarget["tgtBlock"].SourceBlockID)
	assert.Empty(t, byTarget["tgtBlock"].RelationKey)
	assert.Equal(t, "assignee", byTarget["tgtRel"].RelationKey)
	assert.Empty(t, byTarget["tgtRel"].SourceBlockID)
}

func TestDependentObjectLinks_DeterministicRelationOrder(t *testing.T) {
	// given a state with multiple object/file/status/tag relations whose values get
	// emitted alphabetically by relation key — independent of map iteration order.
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*state.State)
	relations := []*model.RelationLink{
		{Key: "company", Format: model.RelationFormat_object},
		{Key: "assignee", Format: model.RelationFormat_object},
		{Key: "author", Format: model.RelationFormat_object},
	}
	st.AddRelationLinks(relations...)
	st.SetDetail("company", domain.StringList([]string{"company1"}))
	st.SetDetail("assignee", domain.StringList([]string{"user1"}))
	st.SetDetail("author", domain.StringList([]string{"author1"}))

	converter := &fakeConverter{}
	fetcher := setupFetcher(t, st.PickRelationLinks())

	var first []OutgoingLink
	for i := 0; i < 20; i++ {
		got := DependentObjectLinks(st, converter, fetcher, Flags{Details: true})
		if i == 0 {
			first = got
			continue
		}
		require.Equal(t, first, got, "iteration %d produced different order", i)
	}
	// expect alphabetical-by-relation-key
	require.Len(t, first, 3)
	assert.Equal(t, "assignee", first[0].RelationKey)
	assert.Equal(t, "author", first[1].RelationKey)
	assert.Equal(t, "company", first[2].RelationKey)
}

func TestDependentObjectLinks_SkipsSelfReferences(t *testing.T) {
	// given a state whose root has a link block, file block, mention mark, and object
	// relation all pointing at the root itself
	objectId := "root"
	st := state.NewDoc(objectId, map[string]simple.Block{
		objectId: simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"l", "f", "t"}}),
		"l":      simple.New(&model.Block{Id: "l", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: objectId}}}),
		"f":      simple.New(&model.Block{Id: "f", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: objectId}}}),
		"t": simple.New(&model.Block{Id: "t", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{Type: model.BlockContentTextMark_Mention, Param: objectId},
				{Type: model.BlockContentTextMark_Object, Param: objectId},
			}},
		}}}),
	}).(*state.State)
	st.AddRelationLinks(&model.RelationLink{Key: "assignee", Format: model.RelationFormat_object})
	st.SetDetail("assignee", domain.StringList([]string{objectId}))

	converter := &fakeConverter{}
	fetcher := setupFetcher(t, st.PickRelationLinks())

	// when
	links := DependentObjectLinks(st, converter, fetcher, Flags{Blocks: true, Details: true})

	// then
	for _, l := range links {
		assert.NotEqual(t, objectId, l.TargetID, "self-reference must be filtered out")
	}
}

func TestDependentObjectLinks_FilterPresentationOnlyDropsIconOnlyReferences(t *testing.T) {
	// given a state where a file id appears ONLY as iconImage
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*state.State)
	st.AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyIconImage.String(),
		Format: model.RelationFormat_file,
	})
	st.SetDetail(bundle.RelationKeyIconImage, domain.StringList([]string{"iconFileId"}))

	converter := &fakeConverter{}
	fetcher := setupFetcher(t, st.PickRelationLinks())

	// when filter is on
	linksFiltered := DependentObjectLinks(st, converter, fetcher, Flags{
		Blocks:                 true,
		Details:                true,
		FilterPresentationOnly: true,
	})
	// when filter is off
	linksAll := DependentObjectLinks(st, converter, fetcher, Flags{
		Blocks:  true,
		Details: true,
	})

	// then
	for _, l := range linksFiltered {
		assert.NotEqual(t, "iconFileId", l.TargetID, "filter should drop icon-only refs")
	}
	hasIcon := false
	for _, l := range linksAll {
		if l.TargetID == "iconFileId" {
			hasIcon = true
		}
	}
	assert.True(t, hasIcon, "without filter the icon ref must still be present")
}

func TestDependentObjectLinks_CollectionStoreEmittedWithoutAttribution(t *testing.T) {
	// given
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*state.State)
	st.UpdateStoreSlice(template.CollectionStoreKey, []string{"obj1", "obj2"})
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, nil)

	// when
	links := DependentObjectLinks(st, converter, fetcher, Flags{
		Blocks:     true,
		Collection: true,
	})

	// then
	require.Len(t, links, 2)
	for _, l := range links {
		assert.Empty(t, l.SourceBlockID)
		assert.Empty(t, l.RelationKey)
		assert.Contains(t, []string{"obj1", "obj2"}, l.TargetID)
	}
}

func TestDependentObjectIDs_CollectionStoreIncluded(t *testing.T) {
	// given a state with two collection store members
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root"}),
	}).(*state.State)
	st.UpdateStoreSlice(template.CollectionStoreKey, []string{"obj1", "obj2"})
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, nil)

	t.Run("Collection flag on: store members appear", func(t *testing.T) {
		ids := DependentObjectIDs(st, converter, fetcher, Flags{Collection: true})
		assert.Subset(t, ids, []string{"obj1", "obj2"})
	})

	t.Run("Collection flag off: store members do NOT appear", func(t *testing.T) {
		ids := DependentObjectIDs(st, converter, fetcher, Flags{})
		assert.NotContains(t, ids, "obj1")
		assert.NotContains(t, ids, "obj2")
	})
}

func TestDependentObjectIDsPerSpace(t *testing.T) {
	// given
	const (
		spc1 = "space1"
		spc2 = "space2"
		spc3 = "space3"
	)
	st := buildStateWithLinks()
	converter := &fakeConverter{}
	fetcher := setupFetcher(t, st.PickRelationLinks())
	resolver := &fakeSpaceIdResolver{idsToSpaceIds: map[string]string{
		"objectID":  spc1,
		"objectID2": spc2,
		"objectID3": spc3,
		"objectID4": spc1,
		"relation1": spc1,
		"relation2": spc1,
		"relation3": spc1,
		"relation4": spc1,
		"relation5": spc1,
		"file":      spc2,
		// "option1": ???,
		"option2": spc2,
		"option3": spc3,
		dateutil.NewDateObject(time.Now(), false).Id(): spc1,
	}}

	// when
	ids := DependentObjectIDsPerSpace(spc1, st, converter, resolver, fetcher, Flags{Blocks: true, Relations: true, Details: true})

	// then
	require.Len(t, ids, 3)
	assert.Len(t, ids[spc1], 9)
	assert.Len(t, ids[spc2], 3)
	assert.Len(t, ids[spc3], 2)
}
