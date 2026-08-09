// Package deletionaudit answers "what was deleted from this space, by whom, and when".
//
// Permanent deletion is destructive on both sides of the store: objecttree storage drops every
// change of the deleted tree (its root change, which carries the creator and creation time,
// included), and the object's index row is replaced by a tombstone. What survives is the space
// settings tree — the space-level log every deletion is recorded in. Each of its changes is signed,
// so it carries the deleter's identity and the deletion time; storage keeps those changes forever,
// since tree reduction only ever prunes the in-memory tree.
//
// This package joins the two halves. It walks the settings tree and stamps deletedBy/deletedDate/
// deletionChangeId onto the matching tombstones, where they sit next to the creation-side relations
// spaceindex.SnapshotOnDelete carried through the wipe. The audit list is then an ordinary index
// query.
//
// The creation side is only as good as what the tombstone kept: objects deleted before tombstone
// preservation shipped have no creator or createdDate to report, and List returns them with those
// fields empty rather than hiding them. The deletion side has no such gap — the settings tree is
// walked from its root, so historical deletions are recovered in full.
package deletionaudit

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"

	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

var log = logging.Logger("deletionaudit")

const CName = "core.block.deletionaudit"

type Service interface {
	app.Component

	// List returns the space's deletion audit records, most recently deleted first. It materializes
	// any settings tree changes seen since the last call before querying, so the result is current.
	//
	// limit == 0 means no limit. total is the number of records matching regardless of limit/offset.
	List(ctx context.Context, spaceId string, limit, offset int) (records []*domain.Details, total int, err error)
}

func New() Service {
	return &service{}
}

type service struct {
	spaceService space.Service
	objectStore  objectstore.ObjectStore
}

func (s *service) Init(a *app.App) error {
	s.spaceService = app.MustComponent[space.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) List(ctx context.Context, spaceId string, limit, offset int) ([]*domain.Details, int, error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, 0, fmt.Errorf("get space: %w", err)
	}
	index := s.objectStore.SpaceIndex(spaceId)

	if err = s.materialize(ctx, spc, index); err != nil {
		// a failed walk costs freshness, not correctness: whatever was materialized by earlier calls
		// is still in the index and still worth returning
		log.With("spaceId", spaceId, "error", err).Error("materialize deletion audit")
	}
	if err = s.materializeUninstalled(ctx, spc, index); err != nil {
		log.With("spaceId", spaceId, "error", err).Error("materialize uninstall audit")
	}

	filters := auditFilters()
	records, err := index.QueryRaw(filters, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query deletion audit records: %w", err)
	}

	// A page that reached the end of the result set already knows the total, so skip the count query.
	// Unbounded, or short of the limit, means the end; requiring a non-empty page (or a zero offset)
	// rules out an offset that overshot, which is indistinguishable from an empty match without
	// counting. Same reasoning as spaceindex.QueryAndCount.
	total := offset + len(records)
	if ((limit != 0) && (len(records) >= limit)) || ((len(records) == 0) && (offset != 0)) {
		total, err = index.CountRaw(filters)
		if err != nil {
			return nil, 0, fmt.Errorf("count deletion audit records: %w", err)
		}
	}

	details := make([]*domain.Details, 0, len(records))
	for _, r := range records {
		details = append(details, r.Details)
	}
	return details, total, nil
}

// auditFilters selects rows this package has materialized, of either kind.
//
// deletedDate is the discriminator: nothing else writes it, so its presence separates a real removal
// from the other reasons a row carries isDeleted — DeleteObject also runs for bundled template
// cleanup and similar index churn, and those ids appear in neither source.
//
// Callers tell the two kinds apart by isUninstalled, which is the object's own detail rather than
// anything this package invents: true means a type/property/template that was uninstalled and can be
// reinstalled, absent means an object destroyed outright.
//
// QueryRaw, not Query: the implicit isDeleted != true filter Query injects would exclude every
// record this method exists to return.
func auditFilters() *database.Filters {
	return &database.Filters{
		FilterObj: database.FiltersAnd{
			database.FilterEq{
				Key:   bundle.RelationKeyIsDeleted,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: domain.Bool(true),
			},
			database.FilterExists{Key: bundle.RelationKeyDeletedDate},
		},
		Order: deletedDateDesc{},
	}
}

// uninstalledFilters selects uninstalled derived objects whose audit fields are still missing.
//
// Missing, not all of them, because the fields do not survive: an uninstalled object keeps its tree,
// so the indexer re-indexes it on the next head change and UpdateObjectDetails replaces the whole row
// with the object's own details, dropping anything this package wrote. Re-deriving only what is
// absent makes that self-healing and keeps the steady-state cost at one query.
func uninstalledFilters() *database.Filters {
	return &database.Filters{
		FilterObj: database.FiltersAnd{
			database.FilterEq{
				Key:   bundle.RelationKeyIsUninstalled,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: domain.Bool(true),
			},
			database.FilterNot{Filter: database.FilterExists{Key: bundle.RelationKeyDeletedDate}},
		},
	}
}

// deletedDateDesc orders newest deletion first. Objects deleted together share one change and so one
// timestamp, hence the id tiebreak: without it their relative order is undefined and a paged read
// could show the same record twice while skipping another.
type deletedDateDesc struct{}

var deletedDateSort = func() query.Sort {
	s, err := query.ParseSort("-"+bundle.RelationKeyDeletedDate.String(), bundle.RelationKeyId.String())
	if err != nil {
		panic(fmt.Errorf("parse deletion audit sort: %w", err))
	}
	return s
}()

func (deletedDateDesc) Compare(a, b *domain.Details) int {
	aDate, bDate := a.GetInt64(bundle.RelationKeyDeletedDate), b.GetInt64(bundle.RelationKeyDeletedDate)
	switch {
	case aDate > bDate:
		return -1
	case aDate < bDate:
		return 1
	}
	aId, bId := a.GetString(bundle.RelationKeyId), b.GetString(bundle.RelationKeyId)
	switch {
	case aId < bId:
		return -1
	case aId > bId:
		return 1
	}
	return 0
}

func (deletedDateDesc) AnystoreSort() query.Sort { return deletedDateSort }

func (deletedDateDesc) UpdateOrderMap([]*domain.Details) bool { return false }

// materialize stamps the deletion facts of every ObjectDelete in the space settings tree onto the
// corresponding tombstone.
//
// The walk always starts at the root. Resuming from the last change of the previous walk would be
// wrong: a settings change synced from another device attaches under its own parent, which can sit
// behind wherever the last walk stopped, and iterating from there would step over it forever. What
// the mark buys instead is skipping the walk entirely while the tree has not grown — the common case
// for an audit view that gets opened, closed and opened again.
//
// Re-applying a change writes the values it wrote before, and ModifyObjectDetailsCtx drops writes
// that change nothing, so a repeated walk costs reads and emits no subscription noise.
func (s *service) materialize(ctx context.Context, spc clientspace.Space, index spaceindex.Store) error {
	settingsId := spc.CommonSpace().Storage().StateStorage().SettingsId()
	if settingsId == "" {
		return fmt.Errorf("space has no settings object")
	}
	// Check the mark before building anything. BuildHistoryTree is BuildFull: it reads every change
	// of the settings tree out of storage and decodes it, which is by far the most expensive thing
	// this package does and is pure waste when nothing has been deleted since the last call. Head
	// storage answers "did this tree move" from one primary-key lookup.
	entry, err := spc.CommonSpace().Storage().HeadStorage().GetEntry(ctx, settingsId)
	if err != nil {
		return fmt.Errorf("get settings head entry: %w", err)
	}
	heads := headsMark(entry.Heads)
	mark, err := index.GetDeletionAuditMark()
	if err != nil {
		return fmt.Errorf("get deletion audit mark: %w", err)
	}
	if (mark != "") && (mark == heads) {
		return nil
	}

	tree, err := spc.TreeBuilder().BuildHistoryTree(ctx, settingsId, objecttreebuilder.HistoryTreeOpts{})
	if err != nil {
		return fmt.Errorf("build settings history tree: %w", err)
	}

	var iterErr error
	err = tree.IterateRoot(unmarshalSettingsData, func(change *objecttree.Change) bool {
		// the root carries the initial space settings, never a deletion
		if len(change.PreviousIds) == 0 {
			return true
		}
		data, ok := change.Model.(*spacesyncproto.SettingsData)
		if !ok {
			return true
		}
		if iterErr = s.applyChange(ctx, index, spc.Id(), change, data); iterErr != nil {
			return false
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("iterate settings tree: %w", err)
	}
	if iterErr != nil {
		return fmt.Errorf("apply settings change: %w", iterErr)
	}

	if err = index.SetDeletionAuditMark(heads); err != nil {
		return fmt.Errorf("save deletion audit mark: %w", err)
	}
	return nil
}

// headsMark renders tree heads as a comparable string. Sorted, because head order is not stable
// across reads and an order flip would look like a changed tree and force a needless walk.
func headsMark(heads []string) string {
	sorted := slices.Clone(heads)
	slices.Sort(sorted)
	return strings.Join(sorted, ",")
}

// materializeUninstalled records who uninstalled each type, property, relation option and template,
// and when.
//
// These never pass through the settings tree. Service.deleteDerivedObject sets isUninstalled = true
// and stops there — it never calls DeleteTree, and the settings object would refuse a derived id
// anyway (ErrCantDeleteDerivedObject). So "delete a type" is not a deletion at all: the tree lives on
// and the object can be reinstalled.
//
// That leaves the evidence in a better state than for a real deletion, not a worse one. isUninstalled
// is an ordinary synced detail, so setting it produced a signed change in the object's own tree, and
// that change's identity and timestamp are exactly who and when. The object also keeps its name,
// which no destroyed object can.
func (s *service) materializeUninstalled(ctx context.Context, spc clientspace.Space, index spaceindex.Store) error {
	records, err := index.QueryRaw(uninstalledFilters(), 0, 0)
	if err != nil {
		return fmt.Errorf("query uninstalled objects: %w", err)
	}
	for _, record := range records {
		objectId := record.Details.GetString(bundle.RelationKeyId)
		if objectId == "" {
			continue
		}
		change, err := s.findUninstallChange(ctx, spc, objectId)
		if err != nil {
			// one unreadable tree must not stop the rest; the row simply stays unmaterialized and is
			// retried on the next call
			log.With("objectId", objectId, "error", err).Warn("find uninstall change")
			continue
		}
		if change == nil {
			continue
		}
		var uninstalledBy string
		if change.Identity != nil {
			uninstalledBy = domain.NewParticipantId(spc.Id(), change.Identity.Account())
		}
		err = index.ModifyObjectDetailsCtx(ctx, objectId, func(details *domain.Details) (*domain.Details, bool, error) {
			if details == nil {
				return nil, false, nil
			}
			details.SetString(bundle.RelationKeyDeletedBy, uninstalledBy)
			details.SetInt64(bundle.RelationKeyDeletedDate, change.Timestamp)
			details.SetString(bundle.RelationKeyDeletionChangeId, change.Id)
			return details, true, nil
		}, false)
		if err != nil {
			return fmt.Errorf("stamp uninstall on %s: %w", objectId, err)
		}
	}
	return nil
}

// findUninstallChange returns the change that last set isUninstalled to true, or nil when the tree
// holds no such change.
//
// Last, not first: uninstalling is reversible — installing a bundled type again sets isUninstalled
// back to false — so a type can cross the line repeatedly and only the most recent crossing describes
// how it got to where it is now.
func (s *service) findUninstallChange(ctx context.Context, spc clientspace.Space, objectId string) (*objecttree.Change, error) {
	tree, err := spc.TreeBuilder().BuildHistoryTree(ctx, objectId, objecttreebuilder.HistoryTreeOpts{})
	if err != nil {
		return nil, fmt.Errorf("build history tree: %w", err)
	}
	// Track the last change that touched the relation at all, in either direction, and report it only
	// if it left the object uninstalled. Tracking only the trues would pin the identity of whoever
	// uninstalled it before the most recent reinstall — an accusation aimed at the wrong person.
	var (
		last      *objecttree.Change
		lastValue bool
	)
	err = tree.IterateRoot(sourceimpl.UnmarshalChange, func(change *objecttree.Change) bool {
		model, ok := change.Model.(*pb.Change)
		if !ok {
			return true
		}
		for _, content := range model.GetContent() {
			set := content.GetDetailsSet()
			if (set == nil) || (set.GetKey() != bundle.RelationKeyIsUninstalled.String()) {
				continue
			}
			last = change
			lastValue = set.GetValue().GetBoolValue()
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("iterate tree: %w", err)
	}
	if !lastValue {
		return nil, nil
	}
	return last, nil
}

// The settings tree is written unencrypted, so IterateFrom hands us the raw change data.
func unmarshalSettingsData(_ *objecttree.Change, decrypted []byte) (any, error) {
	data := &spacesyncproto.SettingsData{}
	if err := data.UnmarshalVT(decrypted); err != nil {
		return nil, fmt.Errorf("unmarshal settings data: %w", err)
	}
	return data, nil
}

// applyChange stamps one settings change onto the tombstones of the objects it deleted.
//
// Only Content is read, never Snapshot: a snapshot restates the accumulated set of deleted ids with
// no idea who deleted each or when, and its own new deletions are in Content alongside it. Reading
// it would attribute other people's deletions to whoever happened to trigger the snapshot.
func (s *service) applyChange(
	ctx context.Context,
	index spaceindex.Store,
	spaceId string,
	change *objecttree.Change,
	data *spacesyncproto.SettingsData,
) error {
	var deletedBy string
	if change.Identity != nil {
		deletedBy = domain.NewParticipantId(spaceId, change.Identity.Account())
	}

	for _, content := range data.GetContent() {
		del := content.GetObjectDelete()
		if del == nil || del.GetId() == "" {
			continue
		}
		err := index.ModifyObjectDetailsCtx(ctx, del.GetId(), func(details *domain.Details) (*domain.Details, bool, error) {
			if details == nil {
				details = domain.NewDetails()
			}
			// upsert: the row is absent when this device never indexed the object (it joined the
			// space after the deletion, say). Write the tombstone so the audit stays complete —
			// the deletion side is all we could have known about it anyway.
			details.SetString(bundle.RelationKeyId, del.GetId())
			details.SetString(bundle.RelationKeySpaceId, spaceId)
			details.SetBool(bundle.RelationKeyIsDeleted, true)
			details.SetString(bundle.RelationKeyDeletedBy, deletedBy)
			details.SetInt64(bundle.RelationKeyDeletedDate, change.Timestamp)
			details.SetString(bundle.RelationKeyDeletionChangeId, change.Id)
			return details, true, nil
		}, true)
		if err != nil {
			return fmt.Errorf("stamp deletion on %s: %w", del.GetId(), err)
		}
	}
	return nil
}
