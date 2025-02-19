package objectlink

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

type fakeConverter struct {
}

func (f *fakeConverter) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	return fakeDerivedID(key.String()), nil
}

func (f *fakeConverter) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	return fakeDerivedID(key.String()), nil
}

type fakeRelationLinksGetter struct {
	data map[string]model.RelationFormat
}

func (g *fakeRelationLinksGetter) GetRelationLink(key string) (relationLink *model.RelationLink, err error) {
	if format, found := g.data[key]; found {
		return &model.RelationLink{Key: key, Format: format}, nil
	}
	rel, err := bundle.GetRelation(domain.RelationKey(key))
	if err != nil {
		return &model.RelationLink{Key: key, Format: model.RelationFormat_object}, nil
	}
	return &model.RelationLink{Key: key, Format: rel.Format}, nil
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

	t.Run("block option is turned on: get ids from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{Blocks: true})
		assert.Len(t, objectIDs, 4)
	})

	t.Run("all options are turned off", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{})
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
	stateWithLinks.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"relation1": domain.String("exe.exe"),
		"relation2": domain.String("tag24"),
		"relation3": domain.String("status_done"),
		"relation4": domain.String("link"),
	}))
	converter := &fakeConverter{}

	relationsMap := map[string]model.RelationFormat{
		"relation1": model.RelationFormat_file,
		"relation2": model.RelationFormat_tag,
		"relation3": model.RelationFormat_status,
		"relation4": model.RelationFormat_object,
	}

	t.Run("blocks option is turned on: get ids from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{Blocks: true})
		assert.Len(t, objectIDs, 11)
	})

	t.Run("dataview only target option is turned on: get only target from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{Blocks: true, DataviewBlockOnlyTarget: true})
		assert.Len(t, objectIDs, 9)
	})

	t.Run("no images option is turned on: get ids from blocks except images", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{Blocks: true, NoImages: true})
		assert.Len(t, objectIDs, 10)
	})

	t.Run("blocks option and relations options are turned on: get ids from blocks and relations", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, &fakeRelationLinksGetter{data: relationsMap}, Flags{Blocks: true, Relations: true})
		assert.Len(t, objectIDs, 15) // 11 links + 4 relations
	})

	t.Run("save backlinks", func(t *testing.T) {
		st := stateWithLinks.Copy()
		st.SetDetail(bundle.RelationKeyBacklinks, domain.StringList([]string{"link1"}))
		objectIDs := DependentObjectIDs(st, converter, &fakeRelationLinksGetter{data: relationsMap}, Flags{Details: true})
		assert.Len(t, objectIDs, 1)
		assert.Contains(t, objectIDs, "link1")
	})
	t.Run("skip backlinks", func(t *testing.T) {
		st := stateWithLinks.Copy()
		st.SetDetail(bundle.RelationKeyBacklinks, domain.StringList([]string{"link1"}))
		objectIDs := DependentObjectIDs(st, converter, &fakeRelationLinksGetter{data: relationsMap}, Flags{Details: true, NoBackLinks: true})
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
	stateWithLinks.SetDetail("relation1", domain.StringList([]string{"file"}))
	stateWithLinks.SetDetail("relation2", domain.StringList([]string{"option1"}))
	stateWithLinks.SetDetail("relation3", domain.StringList([]string{"option2"}))
	stateWithLinks.SetDetail("relation4", domain.StringList([]string{"option3"}))
	stateWithLinks.SetDetail("relation5", domain.Int64(time.Now().Unix()))

	return stateWithLinks
}

func buildRelationsMap() map[string]model.RelationFormat {
	return map[string]model.RelationFormat{
		"relation1": model.RelationFormat_file,
		"relation2": model.RelationFormat_tag,
		"relation3": model.RelationFormat_status,
		"relation4": model.RelationFormat_object,
		"relation5": model.RelationFormat_date,
	}
}

func TestState_DepSmartIdsLinksDetailsAndRelations(t *testing.T) {
	// given
	stateWithLinks := buildStateWithLinks()
	converter := &fakeConverter{}

	t.Run("blocks option is turned on: get ids from blocks", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{Blocks: true})
		assert.Len(t, objectIDs, 4) // links
	})
	t.Run("blocks option and relations option are turned on: get ids from blocks and relations", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, &fakeRelationLinksGetter{}, Flags{Blocks: true, Relations: true})
		assert.Len(t, objectIDs, 9) // 4 links + 5 relations
	})
	t.Run("blocks, relations and details option are turned on: get ids from blocks, relations and details", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, &fakeRelationLinksGetter{buildRelationsMap()}, Flags{Blocks: true, Relations: true, Details: true})
		assert.Len(t, objectIDs, 14) // 4 links + 5 relations + 3 options + 1 fileID + 1 date
	})
}

func TestState_DepSmartIdsLinksCreatorModifierWorkspace(t *testing.T) {
	// given
	stateWithLinks := state.NewDoc("root", nil).(*state.State)
	relationsMap := map[string]model.RelationFormat{
		"relation1": model.RelationFormat_date,
	}
	stateWithLinks.SetDetail("relation1", domain.Int64(time.Now().Unix()))
	stateWithLinks.SetDetail(bundle.RelationKeyCreatedDate, domain.Int64(time.Now().Unix()))
	stateWithLinks.SetDetail(bundle.RelationKeyCreator, domain.String("creator"))
	stateWithLinks.SetDetail(bundle.RelationKeyLastModifiedBy, domain.String("lastModifiedBy"))
	converter := &fakeConverter{}

	t.Run("details option is turned on: get ids only from details", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, &fakeRelationLinksGetter{data: relationsMap}, Flags{Details: true, CreatorModifierWorkspace: true})
		assert.Len(t, objectIDs, 3) // creator + lastModifiedBy + 1 date
	})

	t.Run("details and relations options are turned on: get ids from details and relations", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, &fakeRelationLinksGetter{data: relationsMap}, Flags{Details: true, Relations: true, CreatorModifierWorkspace: true})
		assert.Len(t, objectIDs, 7) // 4 relations + creator + lastModifiedBy + 1 date
	})
}

func TestState_DepSmartIdsObjectTypes(t *testing.T) {
	// given
	stateWithLinks := state.NewDoc("root", nil).(*state.State)
	stateWithLinks.SetObjectTypeKey(bundle.TypeKeyPage)
	converter := &fakeConverter{}

	t.Run("all options are turned off", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{})
		assert.Len(t, objectIDs, 0)
	})
	t.Run("objTypes option is turned on, get only object types id", func(t *testing.T) {
		objectIDs := DependentObjectIDs(stateWithLinks, converter, nil, Flags{Types: true})
		assert.Equal(t, []string{
			fakeDerivedID(bundle.TypeKeyPage.String()),
		}, objectIDs)
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
	ids := DependentObjectIDsPerSpace(spc1, st, converter, resolver,
		&fakeRelationLinksGetter{data: buildRelationsMap()}, Flags{Blocks: true, Relations: true, Details: true})

	// then
	require.Len(t, ids, 3)
	assert.Len(t, ids[spc1], 9)
	assert.Len(t, ids[spc2], 3)
	assert.Len(t, ids[spc3], 2)
}
