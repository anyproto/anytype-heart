// Package persist materializes resolved objects in the target space: creates
// new trees, updates matched existing objects (Revision-guarded), uploads
// file objects, installs bundled dependencies through a coordinator, applies
// favorite/archive flags, and journals every effect for compensation.
package persist

import (
	"context"
	"errors"
	"fmt"

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
	SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool) error
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
	spillDir  string
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

func (p *Persister) persistRegular(ctx context.Context, o *importv2.Object, target Target, report func(importv2.Issue)) (Outcome, error) {
	stampProvenance(o, p.origin)
	ensureRootBlock(o.Payload, target.Id)

	doc, err := state.NewDocFromSnapshot(target.Id, &pb.ChangeSnapshot{Data: o.Payload.ToProto()})
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
		outcome, err = p.createObject(ctx, target, doc)
		if err != nil {
			return Outcome{}, err
		}
	} else if canUpdate(o.SbType) {
		outcome = p.updateObject(target.Id, doc, report)
	}

	p.applyFlags(ctx, o, target.Id, report)
	return outcome, nil
}

func (p *Persister) createObject(ctx context.Context, target Target, doc *state.State) (Outcome, error) {
	sb, err := p.space.CreateTreeObjectWithPayload(ctx, target.Payload, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{
			Ctx:         ctx,
			IsNewObject: true,
			State:       doc,
			SpaceID:     p.spaceId,
		}
	})
	if err == nil {
		p.journal.CreatedObject(target.Id)
		sb.Lock()
		details := sb.Details()
		sb.Unlock()
		return Outcome{Id: target.Id, Action: ActionCreated, Details: details}, nil
	}
	if errors.Is(err, treestorage.ErrTreeExists) {
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
		return Outcome{Id: target.Id, Action: ActionSkipped, Details: details}, nil
	}
	return Outcome{}, fmt.Errorf("create tree object %s: %w", target.Id, err)
}

// updateObject overwrites a matched existing object with the imported state
// (replace semantics — collection membership was already resolved to the
// import's own list). Guarded by Revision so newer bundled objects are never
// downgraded. Failures degrade to Skipped with an issue, as in v1.
func (p *Persister) updateObject(objectId string, doc *state.State, report func(importv2.Issue)) Outcome {
	outcome := Outcome{Id: objectId, Action: ActionSkipped}
	err := cache.Do(p.objects, objectId, func(sb smartblock.SmartBlock) error {
		currentRevision := sb.Details().GetInt64(bundle.RelationKeyRevision)
		if currentRevision > doc.Details().GetInt64(bundle.RelationKeyRevision) {
			return nil // never downgrade (bundled types/relations carry revisions)
		}
		if doc.ObjectTypeKey() == bundle.TypeKeyObjectType {
			template.InitTemplate(doc, template.WithDetail(bundle.RelationKeyRecommendedLayout, domain.Int64(int64(model.ObjectType_basic))))
		}
		if err := history.ResetToVersion(sb, doc); err != nil {
			return fmt.Errorf("reset to imported state: %w", err)
		}
		p.journal.UpdatedObject(objectId)
		outcome = Outcome{Id: objectId, Action: ActionUpdated, Details: sb.CombinedDetails()}
		return nil
	})
	if err != nil {
		report(importv2.Issue{
			Severity: importv2.SeverityWarning,
			Code:     importv2.IssueObjectFailed,
			ObjectId: objectId,
			Message:  "update existing object",
			Err:      err,
		})
	}
	return outcome
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
		if err := p.flags.SetIsArchived(nil, ctx, objectId, true); err != nil {
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
