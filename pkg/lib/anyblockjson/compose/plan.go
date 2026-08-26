package compose

// plan.go — the plan phase of the exporter pipeline (EXPORTER_DESIGN.md
// §1.1): classify every collected document into its kind directory and fix
// every path, single-threaded, from DETAILS-LEVEL facts only, before the
// first emit task starts. Under the settled id naming (design §1.3) a path
// is a pure per-document function of the id — no collision machinery, no
// global set, nothing ordering-sensitive left for the concurrent emit phase
// to disagree about. That purity is the whole determinism argument: same
// space state ⇒ same names, with nothing to prove about scheduling.

import (
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// DocExtension is the document filename suffix, on every kind (design
// §1.3, SPEC §15 #1 settled): the id verbatim plus this. The double
// extension is the entire is-this-a-document test — a FAT bundle carries
// blobs that are themselves .json files, so bare .json cannot be one.
const DocExtension = ".anyblock.json"

// The kind directories of the bundle layout (design §1.2, settled Q1):
// format vocabulary, snake_case, one word each — never the store's
// `relations`/`relationsOptions` spellings, since the format promised the
// word "relation" appears nowhere a reader looks first.
const (
	DirObjects      = "objects"
	DirTypes        = "types"
	DirTemplates    = "templates"
	DirProperties   = "properties"
	DirOptions      = "options"
	DirParticipants = "participants"
	DirFiles        = "files"
)

// DocMeta is what the plan reads about one collected document — details
// only, never content (the invariant that keeps plan O(collected details)
// and free of object loads, design §1.1).
type DocMeta struct {
	Id     string
	SbType model.SmartBlockType
	// FileExt and FileMime are a file object's stored `fileExt` /
	// `fileMimeType` details, raw — the blob path inputs. Ignored for every
	// other kind. Raw because the corpus measured `fileExt` dirty as a path
	// component (431 empty, 12 literally "json", dozens non-alphanumeric);
	// sanitation is BlobExtension's job, not the caller's.
	FileExt  string
	FileMime string
}

// Plan is the deterministic path table: id → document path, and for file
// objects id → blob path, all bundle-relative and slash-separated. A name
// planned for a document the emit then omits simply goes unused —
// determinism is unaffected, since omission is itself a deterministic
// function of state.
type Plan struct {
	docPaths  map[string]string
	blobPaths map[string]string
}

// BuildPlan fixes every path before the first emit task starts. It refuses
// an id that cannot be a filename stem — empty, path separators, a dot-only
// component — because such an id would escape the bundle root; the corpus's
// two id populations (lowercase-base32 CIDs, base58 identities) can never
// trip it, so a refusal here means the store handed us something that is
// not an object id.
func BuildPlan(docs []DocMeta) (*Plan, error) {
	p := &Plan{
		docPaths:  make(map[string]string, len(docs)),
		blobPaths: map[string]string{},
	}
	for _, d := range docs {
		if err := checkIdSafe(d.Id); err != nil {
			return nil, fmt.Errorf("plan document paths: %w", err)
		}
		dir := KindDirectory(d.SbType)
		p.docPaths[d.Id] = dir + "/" + d.Id + DocExtension
		if dir == DirFiles {
			// the blob sits beside its document: same directory, same stem
			// (the id), real sanitized extension — so the two halves of a
			// file sort adjacent in any listing. The manifest `files` map is
			// what BINDS them (§2c); adjacency is this exporter's layout.
			p.blobPaths[d.Id] = dir + "/" + d.Id + "." + BlobExtension(d.FileExt, d.FileMime)
		}
	}
	return p, nil
}

// DocPath is the planned bundle-relative document path for id.
func (p *Plan) DocPath(id string) (string, bool) {
	path, ok := p.docPaths[id]
	return path, ok
}

// BlobPath is the planned bundle-relative blob path for a file object id.
func (p *Plan) BlobPath(id string) (string, bool) {
	path, ok := p.blobPaths[id]
	return path, ok
}

// KindDirectory maps a document's smartblock type onto its kind directory
// (design §1.2). Everything without a dedicated home — pages, the rare
// fail-closed widget or workspace document an omission predicate refuses —
// lands flat in objects/.
func KindDirectory(sbType model.SmartBlockType) string {
	switch sbType {
	case model.SmartBlockType_STType, model.SmartBlockType_BundledObjectType:
		return DirTypes
	case model.SmartBlockType_Template, model.SmartBlockType_BundledTemplate:
		return DirTemplates
	case model.SmartBlockType_STRelation, model.SmartBlockType_BundledRelation:
		return DirProperties
	case model.SmartBlockType_STRelationOption:
		return DirOptions
	case model.SmartBlockType_Participant:
		return DirParticipants
	case model.SmartBlockType_File, model.SmartBlockType_FileObject:
		return DirFiles
	default:
		return DirObjects
	}
}

// blobExtensionByMime is the fallback for a file object whose stored
// `fileExt` is unusable: the conventional extension for the commonest
// stored mime types. A fixed table rather than the platform's mime
// registry, deliberately — mime.ExtensionsByType reads OS files and its
// answer varies by machine, which would make the same space export
// different bytes on different hosts.
var blobExtensionByMime = map[string]string{
	"image/jpeg":         "jpg",
	"image/png":          "png",
	"image/gif":          "gif",
	"image/webp":         "webp",
	"image/svg+xml":      "svg",
	"image/tiff":         "tiff",
	"image/bmp":          "bmp",
	"image/heic":         "heic",
	"application/pdf":    "pdf",
	"text/plain":         "txt",
	"text/csv":           "csv",
	"text/markdown":      "md",
	"application/json":   "json",
	"application/zip":    "zip",
	"video/mp4":          "mp4",
	"video/quicktime":    "mov",
	"audio/mpeg":         "mp3",
	"audio/mp4":          "m4a",
	"audio/wav":          "wav",
	"audio/x-wav":        "wav",
	"application/x-tar":  "tar",
	"application/gzip":   "gz",
	"application/msword": "doc",
}

// BlobExtension sanitizes a file object's stored extension into a safe path
// component (design §1.4 — cosmetic only, since the manifest map binds and
// `file_mime_type` travels in the document): `fileExt` lowercased and
// restricted to [a-z0-9]{1,10}; failing that, the conventional extension
// for `fileMimeType`; failing that, "bin". A computed final suffix of
// "anyblock.json" is impossible by construction — the character class
// admits no dot — so a blob can never impersonate a document.
func BlobExtension(fileExt, fileMime string) string {
	ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(fileExt), "."))
	if isCleanExt(ext) {
		return ext
	}
	if byMime, ok := blobExtensionByMime[strings.ToLower(strings.TrimSpace(fileMime))]; ok {
		return byMime
	}
	return "bin"
}

func isCleanExt(ext string) bool {
	if len(ext) < 1 || len(ext) > 10 {
		return false
	}
	for _, r := range ext {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// checkIdSafe refuses an id that cannot serve as a filename stem inside the
// bundle root. Not a slugging step — ids are written verbatim (design §1.3)
// — just the containment guarantee.
func checkIdSafe(id string) error {
	switch {
	case id == "" || id == "." || id == "..":
		return fmt.Errorf("id %q cannot name a file", id)
	case strings.ContainsAny(id, "/\\\x00"):
		return fmt.Errorf("id %q contains a path separator", id)
	}
	return nil
}
