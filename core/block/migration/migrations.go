package migration

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type SmartBlock interface {
	Id() string
	Type() model.SmartBlockType
	ObjectType() string
}

type MigrationSelector func(sb SmartBlock) bool

func TypeSelector(t bundle.TypeKey) MigrationSelector {
	return func(sb SmartBlock) bool {
		return sb.ObjectType() == t.URL()
	}
}

type Migration struct {
	Version uint32
	Steps   []MigrationStep
}

type MigrationStep struct {
	Selector MigrationSelector
	Proc     func(s *state.State, sb SmartBlock) error
}

func ApplyMigrations(st *state.State, sb SmartBlock) error {
	for _, m := range migrations {
		if st.MigrationVersion() >= m.Version {
			fmt.Println("SKIP", m.Version, "FOR", sb.Id())
			continue
		}
		for _, s := range m.Steps {
			if s.Selector(sb) {
				fmt.Println("APPLY", m.Version, "FOR", sb.Id())
				if err := s.Proc(st, sb); err != nil {
					return fmt.Errorf("MIGRATION FAIL: %w", err)
				}
			}
		}
	}
	return nil
}

var migrations = []Migration{
	{
		Version: 1,
		Steps: []MigrationStep{
			{
				Selector: TypeSelector(bundle.TypeKeyPage),
				Proc: func(s *state.State, sb SmartBlock) error {
					b := simple.New(&model.Block{
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text: "Test 1 " + sb.Id(),
							},
						},
					})

					s.Add(b)

					return s.InsertTo("", model.Block_Inner, b.Model().Id)
				},
			},
		},
	},
	{
		Version: 2,
		Steps: []MigrationStep{
			{
				Selector: TypeSelector(bundle.TypeKeyPage),
				Proc: func(s *state.State, _ SmartBlock) error {
					b := simple.New(&model.Block{
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text: "Second one",
							},
						},
					})

					s.Add(b)

					return s.InsertTo("", model.Block_Inner, b.Model().Id)
				},
			},
		},
	},
}

var lastMigrationVersion uint32

func LastMigrationVersion() uint32 {
	return lastMigrationVersion
}

func init() {
	for _, m := range migrations {
		if lastMigrationVersion < m.Version {
			lastMigrationVersion = m.Version
		}
	}
}
