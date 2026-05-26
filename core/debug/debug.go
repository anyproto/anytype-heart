package debug

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	utilDebug "github.com/anyproto/anytype-heart/util/debug"
)

type debugParticipantSpace struct {
	SpaceId   string `json:"spaceId"`
	SpaceName string `json:"spaceName"`
}

type debugParticipant struct {
	Identity  string                  `json:"identity"`
	Name      string                  `json:"name"`
	IconImage string                  `json:"iconImage"`
	Spaces    []debugParticipantSpace `json:"spaces"`
}

type debugSpaceSystemObjects struct {
	SpaceId   string            `json:"spaceId"`
	SpaceName string            `json:"spaceName"`
	Objects   map[string]string `json:"objects"`
}

func (d *debug) DebugRouter(r chi.Router) {
	r.Get("/participants", utilDebug.JSONHandler(d.debugListParticipants))
	r.Get("/system_objects", utilDebug.JSONHandler(d.debugListSystemObjects))
}

func (d *debug) debugListParticipants(req *http.Request) ([]debugParticipant, error) {
	participantsByIdentity := make(map[string]*debugParticipant)

	err := d.store.IterateSpaceIndex(req.Context(), func(store spaceindex.Store) error {
		records, err := store.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyResolvedLayout,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.Int64(int64(model.ObjectType_participant)),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("query participants in space %s: %w", store.SpaceId(), err)
		}

		spaceId := store.SpaceId()
		spaceName := d.store.GetSpaceName(spaceId)

		for _, rec := range records {
			identity := rec.Details.GetString(bundle.RelationKeyIdentity)
			if identity == "" {
				continue
			}

			p, ok := participantsByIdentity[identity]
			if !ok {
				p = &debugParticipant{
					Identity:  identity,
					Name:      rec.Details.GetString(bundle.RelationKeyName),
					IconImage: rec.Details.GetString(bundle.RelationKeyIconImage),
				}
				participantsByIdentity[identity] = p
			}

			p.Spaces = append(p.Spaces, debugParticipantSpace{
				SpaceId:   spaceId,
				SpaceName: spaceName,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate space indexes: %w", err)
	}

	result := make([]debugParticipant, 0, len(participantsByIdentity))
	for _, p := range participantsByIdentity {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (d *debug) debugListSystemObjects(req *http.Request) ([]debugSpaceSystemObjects, error) {
	var result []debugSpaceSystemObjects

	err := d.store.IterateSpaceIndex(req.Context(), func(store spaceindex.Store) error {
		spaceId := store.SpaceId()
		spc, err := d.spaceService.Get(context.Background(), spaceId)
		if err != nil {
			return nil
		}

		ids := spc.DerivedIDs()
		spaceName := d.store.GetSpaceName(spaceId)

		objects := map[string]string{}
		if ids.Home != "" {
			objects["home"] = ids.Home
		}
		if ids.Archive != "" {
			objects["archive"] = ids.Archive
		}
		if ids.Widgets != "" {
			objects["widgets"] = ids.Widgets
		}
		if ids.Workspace != "" {
			objects["workspace"] = ids.Workspace
		}
		if ids.Profile != "" {
			objects["profile"] = ids.Profile
		}
		if ids.SpaceChat != "" {
			objects["spaceChat"] = ids.SpaceChat
		}

		result = append(result, debugSpaceSystemObjects{
			SpaceId:   spaceId,
			SpaceName: spaceName,
			Objects:   objects,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate space indexes: %w", err)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SpaceName < result[j].SpaceName
	})

	return result, nil
}
