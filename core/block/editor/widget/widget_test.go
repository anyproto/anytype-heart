package widget

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestCalculateTargetAndPosition(t *testing.T) {
	tests := []struct {
		name           string
		targetId       string
		setupBlocks    map[string]simple.Block
		expectedTarget string
		expectedPos    model.BlockPosition
		expectedError  error
	}{
		{
			name:     "DefaultWidgetFavorite with no root children",
			targetId: DefaultWidgetFavorite,
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{},
				}),
			},
			expectedTarget: "",
			expectedPos:    model.Block_Bottom,
			expectedError:  nil,
		},
		{
			name:     "DefaultWidgetFavorite with root children",
			targetId: DefaultWidgetFavorite,
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{"child1", "child2"},
				}),
				"child1": base.NewBase(&model.Block{
					Id: "child1",
				}),
				"child2": base.NewBase(&model.Block{
					Id: "child2",
				}),
			},
			expectedTarget: "child1",
			expectedPos:    model.Block_Top,
			expectedError:  nil,
		},
		{
			name:     "Non-favorite widget with no existing blocks",
			targetId: "custom-widget",
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{},
				}),
			},
			expectedTarget: "",
			expectedPos:    model.Block_Bottom,
			expectedError:  nil,
		},
		{
			name:     "Widget already exists - should return error",
			targetId: "custom-widget",
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{"wrapper1"},
				}),
				"wrapper1": base.NewBase(&model.Block{
					Id:          "wrapper1",
					ChildrenIds: []string{"link1"},
				}),
				"link1": base.NewBase(&model.Block{
					Id: "link1",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: "custom-widget",
						},
					},
				}),
			},
			expectedTarget: "",
			expectedPos:    0,
			expectedError:  ErrWidgetAlreadyExists,
		},
		{
			name:     "Bin widget exists and is last - should insert above bin",
			targetId: "custom-widget",
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{"wrapper1", "binWrapper"},
				}),
				"wrapper1": base.NewBase(&model.Block{
					Id:          "wrapper1",
					ChildrenIds: []string{"otherLink"},
				}),
				"otherLink": base.NewBase(&model.Block{
					Id: "otherLink",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: "other-widget",
						},
					},
				}),
				"binWrapper": base.NewBase(&model.Block{
					Id:          "binWrapper",
					ChildrenIds: []string{"binLink"},
				}),
				"binLink": base.NewBase(&model.Block{
					Id: "binLink",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: DefaultWidgetBin,
						},
					},
				}),
			},
			expectedTarget: "binWrapper",
			expectedPos:    model.Block_Top,
			expectedError:  nil,
		},
		{
			name:     "Bin widget exists but is not last - should append to bottom",
			targetId: "custom-widget",
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{"binWrapper", "wrapper1"},
				}),
				"binWrapper": base.NewBase(&model.Block{
					Id:          "binWrapper",
					ChildrenIds: []string{"binLink"},
				}),
				"binLink": base.NewBase(&model.Block{
					Id: "binLink",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: DefaultWidgetBin,
						},
					},
				}),
				"wrapper1": base.NewBase(&model.Block{
					Id:          "wrapper1",
					ChildrenIds: []string{"otherLink"},
				}),
				"otherLink": base.NewBase(&model.Block{
					Id: "otherLink",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: "other-widget",
						},
					},
				}),
			},
			expectedTarget: "",
			expectedPos:    model.Block_Bottom,
			expectedError:  nil,
		},
		{
			name:     "Empty root with bin widget - bin not in root children",
			targetId: "custom-widget",
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{},
				}),
				"binWrapper": base.NewBase(&model.Block{
					Id:          "binWrapper",
					ChildrenIds: []string{"binLink"},
				}),
				"binLink": base.NewBase(&model.Block{
					Id: "binLink",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: DefaultWidgetBin,
						},
					},
				}),
			},
			expectedTarget: "",
			expectedPos:    model.Block_Bottom,
			expectedError:  nil,
		},
		{
			name:     "Multiple widgets with no bin widget",
			targetId: "new-widget",
			setupBlocks: map[string]simple.Block{
				"root": base.NewBase(&model.Block{
					Id:          "root",
					ChildrenIds: []string{"wrapper1", "wrapper2"},
				}),
				"wrapper1": base.NewBase(&model.Block{
					Id:          "wrapper1",
					ChildrenIds: []string{"link1"},
				}),
				"link1": base.NewBase(&model.Block{
					Id: "link1",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: "widget1",
						},
					},
				}),
				"wrapper2": base.NewBase(&model.Block{
					Id:          "wrapper2",
					ChildrenIds: []string{"link2"},
				}),
				"link2": base.NewBase(&model.Block{
					Id: "link2",
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: "widget2",
						},
					},
				}),
			},
			expectedTarget: "",
			expectedPos:    model.Block_Bottom,
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			st := state.NewDoc("root", tt.setupBlocks).NewState()

			// when
			target, pos, err := calculateTargetAndPosition(st, tt.targetId)

			// then
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTarget, target)
				assert.Equal(t, tt.expectedPos, pos)
			}
		})
	}
}
