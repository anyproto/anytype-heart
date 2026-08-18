// Package persist materializes resolved objects in the target space: creates
// new trees, updates matched existing objects (Revision-guarded), uploads
// file objects, installs bundled dependencies through a coordinator, applies
// favorite/archive flags, and journals every effect for compensation.
package persist

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("import-v2-persist")

// Space is the clientspace subset used for object materialization.
type Space interface {
	CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error)
	Do(objectId string, apply func(sb smartblock.SmartBlock) error) error
}

// ObjectAccess opens and deletes existing objects (update + compensation).
type ObjectAccess interface {
	cache.ObjectGetter
	DeleteObject(objectId string) error
}

// FileUploader uploads file bytes (path or url) or registers already
// content-addressed files. Satisfied by block.Service + fileobject.Service.
type FileUploader interface {
	UploadFile(ctx context.Context, spaceId string, req block.FileUploadRequest) (objectId string, fileType model.BlockContentFileType, details *domain.Details, err error)
	CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin, additionalDetails *domain.Details) (string, error)
}

// FlagSetter applies favorite/archive outside the object state.
type FlagSetter interface {
	SetIsFavorite(objectId string, isFavorite bool) error
	SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool, skipCascade bool) error
}

// ObjectChecker probes whether an id was already indexed in the space —
// the upload-time classification of content-deduped file objects (a
// pre-existing object is indexed long before the run; a just-created one is
// not yet). Misclassification biases toward "pre-existing", i.e. toward
// leaking a file on abort rather than deleting user data.
type ObjectChecker interface {
	Exists(id string) bool
}

// StateRewriter is the resolver seam: rewrites all references in place.
type StateRewriter interface {
	RewriteState(ctx context.Context, st *state.State, report func(importv2.Issue)) error
}

// Target is the identity decision for the object being persisted.
type Target struct {
	Id         string
	IsExisting bool
	Payload    treestorage.TreeStorageCreatePayload
}

// Action describes what Persist did with one object.
type Action int

const (
	ActionCreated Action = iota
	ActionUpdated
	ActionSkipped
)

type Outcome struct {
	Id      string
	Action  Action
	Details *domain.Details
}

type Persister struct {
	spaceId   string
	origin    objectorigin.ObjectOrigin
	space     Space
	objects   ObjectAccess
	uploader  FileUploader
	flags     FlagSetter
	rewriter  StateRewriter
	installer *InstallCoordinator
	journal   *Journal
	checker   ObjectChecker
	spillDir  string
	heal      func(sourceKey string, derived bool) bool

	typesMu      sync.Mutex
	createdTypes []string
}

// TypeReconciler is the object-type editor's seam: a type brings its own
// dataview back in line with the properties it recommends.
type TypeReconciler interface {
	ReconcileDataviewColumns() error
}

// ReconcileTypes settles the dataview of every type this run created. Workers
// persist objects concurrently, so a type can be created while one of its
// relations is still being written: the dataview template resolves relation
// ids through the store, and a property whose relation is not there yet is
// simply missing from the view — a column the user never gets. By the time
// the run ends every relation exists, and this is where the types catch up.
//
// Best effort by design: a type that cannot be opened keeps the view it has,
// which the editor reconciles on its next load anyway.
func (p *Persister) ReconcileTypes(ctx context.Context) {
	p.typesMu.Lock()
	ids := p.createdTypes
	p.createdTypes = nil
	p.typesMu.Unlock()

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return
		}
		err := p.space.Do(id, func(sb smartblock.SmartBlock) error {
			reconciler, ok := sb.(TypeReconciler)
			if !ok {
				return nil
			}
			return reconciler.ReconcileDataviewColumns()
		})
		if err != nil {
			log.With("objectId", id).Warnf("reconcile type dataview: %v", err)
		}
	}
}

func (p *Persister) noteCreatedType(o *importv2.Object, outcome Outcome) {
	if o.SbType != coresb.SmartBlockTypeObjectType || outcome.Action == ActionSkipped {
		return
	}
	p.typesMu.Lock()
	p.createdTypes = append(p.createdTypes, outcome.Id)
	p.typesMu.Unlock()
}

// SetResumeHeal installs the resumed-incarnation ErrTreeExists policy
// (DM spec §8.1; 08-13 §6.2, D4): heal reports whether the ledger proves
// THIS run created the colliding tree (a non-terminal minted claim, or a
// non-terminal derived intent row — an interrupted create whose tree may
// be hollow). The proof is CLASS-GUARDED (review Class C): a key is
// healable only against proof of its own class, so a converter claiming a
// key it later emits as a derived object can never turn minted proof into
// a ResetToVersion of a pre-existing user object. Everything unproven
// keeps the plain skip-and-read fallback. nil (first incarnations) keeps
// the fallback everywhere.
func (p *Persister) SetResumeHeal(heal func(sourceKey string, derived bool) bool) {
	p.heal = heal
}

func New(
	spaceId string,
	origin objectorigin.ObjectOrigin,
	space Space,
	objects ObjectAccess,
	uploader FileUploader,
	flags FlagSetter,
	rewriter StateRewriter,
	installer *InstallCoordinator,
	journal *Journal,
	checker ObjectChecker,
	spillDir string,
) *Persister {
	return &Persister{
		spaceId:   spaceId,
		origin:    origin,
		space:     space,
		objects:   objects,
		uploader:  uploader,
		flags:     flags,
		rewriter:  rewriter,
		installer: installer,
		journal:   journal,
		checker:   checker,
		spillDir:  spillDir,
	}
}

// Persist materializes one resolved object. Issues that do not fail the
// object (missing refs, flag failures) go to report; the returned error means
// the object itself failed.
func (p *Persister) Persist(ctx context.Context, o *importv2.Object, target Target, report func(importv2.Issue)) (Outcome, error) {
	switch o.SbType {
	case coresb.SmartBlockTypeFileObject, coresb.SmartBlockTypeFile:
		return p.persistFile(ctx, o)
	case coresb.SmartBlockTypeWorkspace, coresb.SmartBlockTypeWidget:
		// Only anytype-export archives carry these; they arrive with the pb
		// converter phase. A md/notion converter emitting them is a bug.
		return Outcome{}, importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueInvariant,
			SourceKey: o.SourceKey,
			Message:   fmt.Sprintf("smartblock type %s is not supported by import v2 yet", o.SbType),
		}
	default:
		return p.persistRegular(ctx, o, target, report)
	}
}

// maxSnapshotBytes bounds one object's snapshot change. Node deployments
// have historically enforced a 64MB message ceiling (GO-1433) — a change
// above it persists locally but can never replicate; 48MB leaves headroom
// for the change envelope and encryption overhead. Better one loud
// per-object failure than a silently sync-dead object.
const maxSnapshotBytes = 48 << 20

func (p *Persister) persistRegular(ctx context.Context, o *importv2.Object, target Target, report func(importv2.Issue)) (Outcome, error) {
	stampProvenance(o, p.origin)
	ensureRootBlock(o.Payload, target.Id)

	snapshot := &pb.ChangeSnapshot{Data: o.Payload.ToProto()}
	if size := snapshot.Size(); size > maxSnapshotBytes {
		return Outcome{}, importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueObjectTooLarge,
			SourceKey: o.SourceKey,
			Message: fmt.Sprintf("object is %d MB — larger than the %d MB single-object sync ceiling; split the source document",
				size>>20, maxSnapshotBytes>>20),
		}
	}
	doc, err := state.NewDocFromSnapshot(target.Id, snapshot)
	if err != nil {
		return Outcome{}, fmt.Errorf("doc from snapshot: %w", err)
	}
	doc.SetLocalDetail(bundle.RelationKeyLastModifiedDate, o.Payload.Details.Get(bundle.RelationKeyLastModifiedDate))

	if err := p.rewriter.RewriteState(ctx, doc, report); err != nil {
		return Outcome{}, fmt.Errorf("resolve references: %w", err)
	}
	p.installBundledDeps(ctx, o, doc, report)

	outcome := Outcome{Id: target.Id, Action: ActionSkipped}
	if target.Payload.RootRawChange != nil {
		derived := importv2.IsDerivedClass(o.SbType)
		if derived {
			// Write-ahead intent (review Class C): derived-class objects
			// have no pass-1 claim, so without this row a create torn
			// between the tree write and its effect row leaves NO record —
			// unhealable, uncompensable, silently hollow. Failure aborts
			// (§7.2: a run that cannot journal must not create objects).
			if err = p.journal.CreateIntent(o.SourceKey, target.Id); err != nil {
				return Outcome{}, err
			}
		}
		outcome, err = p.createObject(ctx, o.SourceKey, target, doc, derived)
		if err != nil {
			return Outcome{}, err
		}
	} else if canUpdate(o.SbType) {
		outcome, err = p.updateObject(o.SourceKey, target.Id, doc, report)
		if err != nil {
			return Outcome{}, err
		}
	}

	p.applyFlags(ctx, o, target.Id, report)
	p.noteCreatedType(o, outcome)
	return outcome, nil
}

func (p *Persister) createObject(ctx context.Context, sourceKey string, target Target, doc *state.State, derived bool) (Outcome, error) {
	sb, err := p.space.CreateTreeObjectWithPayload(ctx, target.Payload, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{
			Ctx:         ctx,
			IsNewObject: true,
			State:       doc,
			SpaceID:     p.spaceId,
		}
	})
	if err == nil {
		if err = p.journal.CreatedObject(sourceKey, target.Id); err != nil {
			// The tree exists but its durable record failed: abort (§7.2).
			// The in-memory record stays, so compensation still covers it.
			return Outcome{}, err
		}
		sb.Lock()
		details := sb.Details()
		sb.Unlock()
		return Outcome{Id: target.Id, Action: ActionCreated, Details: details}, nil
	}
	if errors.Is(err, treestorage.ErrTreeExists) {
		if p.heal != nil && p.heal(sourceKey, derived) {
			// Resumed incarnation with ledger proof. For MINTED ids the
			// proof is sound on its own (the id is seed-random — only this
			// run can have written the tree). For DERIVED ids it is NOT:
			// the id is deterministic and the intent row precedes the
			// create, so an intent interrupted BEFORE the tree write reads
			// identically to one torn AFTER it — and the colliding tree may
			// belong to an EARLIER import. The tree itself disambiguates:
			// Class C's tear leaves a HOLLOW tree (root written, state
			// never applied), while a pre-existing object is fully formed.
			// Heal only the hollow; a formed collision resolves as matched
			// below (worst case an ours-but-completed tree leaks one object
			// on a later abort — the leak-bias direction).
			if !derived {
				return p.healInterruptedCreate(sourceKey, target.Id, doc)
			}
			if hollow, hollowErr := p.isHollow(target.Id); hollowErr == nil && hollow {
				return p.healInterruptedCreate(sourceKey, target.Id, doc)
			}
		}
		// An index-invisible tree already exists (prior run, index lag):
		// read it instead of failing so re-import stays idempotent.
		var details *domain.Details
		err = p.space.Do(target.Id, func(sb smartblock.SmartBlock) error {
			details = sb.Details()
			return nil
		})
		if err != nil {
			return Outcome{}, fmt.Errorf("read existing object %s: %w", target.Id, err)
		}
		if derived {
			// Resolve the write-ahead intent: a DERIVED id is deterministic,
			// so this collision means the object PRE-EXISTS this run (an
			// earlier import made it) — the intent row must not keep reading
			// as ownership, or a later resume "heals" (resets) and a later
			// compensation DELETES another run's object off false proof.
			if err = p.journal.SkippedExisting(sourceKey, target.Id); err != nil {
				return Outcome{}, err
			}
		}
		return Outcome{Id: target.Id, Action: ActionSkipped, Details: details}, nil
	}
	return Outcome{}, fmt.Errorf("create tree object %s: %w", target.Id, err)
}

// isHollow reports whether the tree carries no applied state — the shape a
// create torn between the tree write and the state application leaves.
func (p *Persister) isHollow(objectId string) (bool, error) {
	hollow := false
	err := cache.Do(p.objects, objectId, func(sb smartblock.SmartBlock) error {
		details := sb.Details()
		hollow = details == nil || details.Len() == 0
		return nil
	})
	return hollow, err
}

// healInterruptedCreate applies the imported state over a tree this run's
// previous incarnation created but may not have finished initializing, and
// records the CREATE the crash interrupted: the ledger's mode proves the
// run made the object, so ActionSkipped would corrupt compensation
// attribution. No revision guard: the target is provably this run's own
// unfinished object, not a matched existing one.
func (p *Persister) healInterruptedCreate(sourceKey, objectId string, doc *state.State) (Outcome, error) {
	var details *domain.Details
	err := cache.Do(p.objects, objectId, func(sb smartblock.SmartBlock) error {
		if err := history.ResetToVersion(sb, doc); err != nil {
			return fmt.Errorf("reset to imported state: %w", err)
		}
		details = sb.CombinedDetails()
		return nil
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("heal interrupted create %s: %w", objectId, err)
	}
	if err = p.journal.CreatedObject(sourceKey, objectId); err != nil {
		return Outcome{}, err
	}
	return Outcome{Id: objectId, Action: ActionCreated, Details: details}, nil
}

// updateObject overwrites a matched existing object with the imported state
// (replace semantics — collection membership was already resolved to the
// import's own list). Guarded by Revision so newer bundled objects are never
// downgraded. Update failures degrade to Skipped with an issue, as in v1;
// the returned error is reserved for a journal (durable ledger) failure,
// which must abort instead of degrading.
func (p *Persister) updateObject(sourceKey, objectId string, doc *state.State, report func(importv2.Issue)) (Outcome, error) {
	outcome := Outcome{Id: objectId, Action: ActionSkipped}
	err := cache.Do(p.objects, objectId, func(sb smartblock.SmartBlock) error {
		currentRevision := sb.Details().GetInt64(bundle.RelationKeyRevision)
		if currentRevision > doc.Details().GetInt64(bundle.RelationKeyRevision) {
			return nil // never downgrade (bundled types/relations carry revisions)
		}
		if doc.ObjectTypeKey() == bundle.TypeKeyObjectType {
			// A same-named type the USER authored (not import-created, not
			// bundled) is reused, never rewritten: imports may only redefine
			// types that imports created. Matters doubly for LLM-planned
			// types, whose names come from an untrusted source.
			existingOrigin := sb.Details().GetInt64(bundle.RelationKeyOrigin)
			if currentRevision == 0 && existingOrigin != int64(model.ObjectOrigin_import) {
				return nil
			}
			template.InitTemplate(doc, template.WithDetail(bundle.RelationKeyRecommendedLayout, domain.Int64(int64(model.ObjectType_basic))))
		}
		if err := history.ResetToVersion(sb, doc); err != nil {
			return fmt.Errorf("reset to imported state: %w", err)
		}
		outcome = Outcome{Id: objectId, Action: ActionUpdated, Details: sb.CombinedDetails()}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// The reset died OF a stop: propagate so process()'s absorb (or
			// stageInterrupted) classifies it as the stop. Degrading it to a
			// warning here recorded the shutdown as a durable user-facing
			// issue, which a resumed run then rehydrated into its report
			// (found by the update-crash harness variant, review Class H).
			return Outcome{}, err
		}
		report(importv2.Issue{
			Severity: importv2.SeverityWarning,
			Code:     importv2.IssueObjectFailed,
			ObjectId: objectId,
			Message:  "update existing object",
			Err:      err,
		})
		return outcome, nil
	}
	if outcome.Action == ActionUpdated {
		if err = p.journal.UpdatedObject(sourceKey, objectId); err != nil {
			return Outcome{}, err
		}
	}
	return outcome, nil
}

func (p *Persister) installBundledDeps(ctx context.Context, o *importv2.Object, doc *state.State, report func(importv2.Issue)) {
	typeKeys := doc.ObjectTypeKeys()
	if o.SbType == coresb.SmartBlockTypeObjectType {
		// widen so bundled templates of an imported type install too
		typeKeys = append(typeKeys, domain.TypeKey(doc.UniqueKeyInternal()))
	}
	ids := make([]string, 0, len(typeKeys))
	for _, key := range doc.AllRelationKeys() {
		if bundle.HasRelation(key) {
			ids = append(ids, key.BundledURL())
		}
	}
	for _, typeKey := range typeKeys {
		if bundle.HasObjectTypeByKey(typeKey) {
			ids = append(ids, typeKey.BundledURL())
		}
	}
	if err := p.installer.Ensure(ctx, ids); err != nil {
		// Installs are best-effort (v1 parity): the object still imports,
		// missing bundled deps self-heal on next use.
		report(importv2.Issue{
			Severity:  importv2.SeverityWarning,
			Code:      importv2.IssueStoreError,
			SourceKey: o.SourceKey,
			Message:   "install bundled dependencies",
			Err:       err,
		})
	}
}

func (p *Persister) applyFlags(ctx context.Context, o *importv2.Object, objectId string, report func(importv2.Issue)) {
	if o.Favorite {
		if err := p.flags.SetIsFavorite(objectId, true); err != nil {
			report(flagIssue(objectId, "favorite", err))
		}
	}
	if o.Archived {
		if err := p.flags.SetIsArchived(nil, ctx, objectId, true, true); err != nil {
			report(flagIssue(objectId, "archived", err))
		}
	}
}

func flagIssue(objectId, flag string, err error) importv2.Issue {
	return importv2.Issue{
		Severity: importv2.SeverityWarning,
		Code:     importv2.IssueObjectFailed,
		ObjectId: objectId,
		Message:  "set " + flag,
		Err:      err,
	}
}

func canUpdate(sbType coresb.SmartBlockType) bool {
	return sbType != coresb.SmartBlockTypeRelation &&
		sbType != coresb.SmartBlockTypeRelationOption &&
		sbType != coresb.SmartBlockTypeFileObject &&
		sbType != coresb.SmartBlockTypeParticipant
}

// stampProvenance writes origin/import-type and normalizes the created/
// modified timestamps (never falling back to time.Now for the created date —
// it must stay consistent with the tree header).
func stampProvenance(o *importv2.Object, origin objectorigin.ObjectOrigin) {
	details := o.Payload.Details
	createdDate := details.GetInt64(bundle.RelationKeyCreatedDate)
	if details.GetInt64(bundle.RelationKeyLastModifiedDate) == 0 {
		if createdDate != 0 {
			details.SetInt64(bundle.RelationKeyLastModifiedDate, createdDate)
		} else {
			log.With("sourceKey", o.SourceKey).Warnf("imported object carries neither created nor modified date")
		}
	}
	if createdDate > 0 {
		o.Payload.OriginalCreatedTimestamp = createdDate
	}
	details.SetInt64(bundle.RelationKeyOrigin, int64(origin.Origin))
	details.SetInt64(bundle.RelationKeyImportType, int64(origin.ImportType))
}

func ensureRootBlock(payload *importv2.Snapshot, objectId string) {
	for _, b := range payload.Blocks {
		if b.Id == objectId {
			return
		}
	}
	payload.Blocks = anymark.AddRootBlock(payload.Blocks, objectId)
}
