package debug

import (
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

func (d *debug) DebugRouter(r chi.Router) {
	r.Get("/participants", utilDebug.JSONHandler(d.debugListParticipants))
}

func (d *debug) debugListParticipants(req *http.Request) ([]debugParticipant, error) {
	participantsByIdentity := make(map[string]*debugParticipant)

	err := d.store.IterateSpaceIndex(func(store spaceindex.Store) error {
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
