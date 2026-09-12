package importv2

import (
	"context"
	"io"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Object is the unit that flows converter → resolver → persister. A converter
// emits objects one at a time through a Sink and must drop its reference once
// Sink.Object returns; the engine owns the object from then on.
//
// All cross-object references inside Payload (link blocks, mention/object
// marks, object-format detail values, collection membership) use SourceKeys.
// The resolver rewrites them to final ids; converters never see final ids.
// Bundled relation/type URLs and date object ids are preserved as-is.
type Object struct {
	// SourceKey is the converter-scoped stable identity of this object in the
	// source: a normalized file path (Markdown), a Notion page/database id, a
	// derived key for relations/options, etc. Must be unique within a run.
	SourceKey string

	SbType coresb.SmartBlockType

	// Payload is the state snapshot in domain types, structurally identical
	// to model.SmartBlockSnapshotBase. Never nil.
	Payload *Snapshot

	// File is set for file objects only and describes where the bytes are.
	File *FileSource

	// IsRootCandidate marks the object as a member of the run's root
	// collection (subject to the converter's RootSpec and request options).
	IsRootCandidate bool

	// Favorite and Archived are applied by the persister via the details
	// service after creation; they must not be present in Payload details.
	Favorite bool
	Archived bool
}

// Snapshot is the v2 mirror of model.SmartBlockSnapshotBase in domain types
// (same shape as v1's common.StateSnapshot, redefined here so v1 can be
// removed without breaking v2).
type Snapshot struct {
	Blocks                   []*model.Block
	Details                  *domain.Details
	FileKeys                 *types.Struct
	ExtraRelations           []*model.Relation
	ObjectTypes              []string
	Collections              *types.Struct
	RemovedCollectionKeys    []string
	RelationLinks            []*model.RelationLink
	Key                      string
	OriginalCreatedTimestamp int64
	FileInfo                 *model.FileInfo
}

// ToProto converts the snapshot to its wire form. Details may be nil.
func (s *Snapshot) ToProto() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks:                   s.Blocks,
		Details:                  s.Details.ToProto(),
		FileKeys:                 s.FileKeys,
		ExtraRelations:           s.ExtraRelations,
		ObjectTypes:              s.ObjectTypes,
		Collections:              s.Collections,
		RemovedCollectionKeys:    s.RemovedCollectionKeys,
		RelationLinks:            s.RelationLinks,
		Key:                      s.Key,
		OriginalCreatedTimestamp: s.OriginalCreatedTimestamp,
		FileInfo:                 s.FileInfo,
	}
}

// NewSnapshotFromProto converts a wire snapshot into domain form.
func NewSnapshotFromProto(s *model.SmartBlockSnapshotBase) *Snapshot {
	return &Snapshot{
		Blocks:                   s.Blocks,
		Details:                  domain.NewDetailsFromProto(s.Details),
		FileKeys:                 s.FileKeys,
		ExtraRelations:           s.ExtraRelations,
		ObjectTypes:              s.ObjectTypes,
		Collections:              s.Collections,
		RemovedCollectionKeys:    s.RemovedCollectionKeys,
		RelationLinks:            s.RelationLinks,
		Key:                      s.Key,
		OriginalCreatedTimestamp: s.OriginalCreatedTimestamp,
		FileInfo:                 s.FileInfo,
	}
}

// FileSource describes file bytes without holding them. Exactly one of Path
// or Open must be set. Open lets archive-backed converters stream the entry on
// demand; the engine spills to a sanitized temp file only if the uploader
// needs a real path.
type FileSource struct {
	Path string
	Open func(ctx context.Context) (io.ReadCloser, error)

	// Name is the original file name (required when Open is used; the
	// uploader derives it from Path otherwise).
	Name string

	// EncryptionKeys are per-path keys registered with the upload for
	// already-encrypted content (anytype exports). Empty for md/notion —
	// NO shipped converter sets this yet (or ImageKind): both are reserved
	// for the pb/anytype-export converter, their spool serialization is
	// pinned (runstore round-trip test), and consumer-side test coverage
	// is owed by whichever converter first populates them (recorded per
	// review: plumbing must not LOOK covered).
	EncryptionKeys map[string]string

	// URL is the original remote location, for provenance and diagnostics.
	URL string

	// OwnerSourceKey names the object the file was created in, and OwnerRef
	// the block id (file blocks) or relation key (icon, cover, file-valued
	// property) that holds the reference. The persister resolves the key to
	// its final id and hands both to the upload as createdInContext /
	// createdInContextRef, which is what object GC reads to decide whether a
	// file is still owned. A file emitted by more than one reference keeps
	// the first owner (converters emit each file once).
	//
	// Both empty means no owner, and that is the honest value whenever the
	// reference produces no link row for the indexer to turn into a backlink
	// (a callout's text.IconImage) or the owner cannot own orphans (a derived
	// object type): object GC judges "still referenced?" by backlinks, so a
	// context those rules cannot back would offer the user a file in use.
	// Content-addressed registrations (anytype exports) ignore these fields —
	// persistContentAddressedFile does not upload, and file ownership there is
	// not yet carried at all.
	OwnerSourceKey string
	OwnerRef       string

	ImageKind model.ImageKind
}

// IsDerivedClass reports whether the type's identity derives from a unique
// key on demand (relation, option, type, profile): never claimed in pass 1,
// deterministic tree derivation, re-derived on a resumed replay. ONE
// predicate — the engine's sink routing and persist's write-ahead intent
// must never disagree about the class.
func IsDerivedClass(sbType coresb.SmartBlockType) bool {
	switch sbType {
	case coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption,
		coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeProfilePage:
		return true
	default:
		return false
	}
}

// IsFileClass reports the file lane's class (upload path, id via future,
// never a pass-1 claim). ONE predicate, like IsDerivedClass above: the
// engine's lane routing and the crawl loader's claim/spool cross-check must
// never disagree about which spool rows require a claim row.
func IsFileClass(sbType coresb.SmartBlockType) bool {
	return sbType == coresb.SmartBlockTypeFileObject || sbType == coresb.SmartBlockTypeFile
}
