# Context-Aware GC — Stage 2: All Objects

**Related:** GO-6052 (Stage 1 — files only), PR #2889

## Background

Stage 1 introduced `createdInContext` / `createdInContextRef` relations on file objects and a GC service (`core/files/filegc`) that archives orphaned files when their creation-context parent is archived. Every GC query was filtered to `domain.FileLayouts` only.

Stage 2 extends the same mechanism to all user-content objects. When you create an object inside another object via `BlockLinkCreateWithObject`, that relationship is tracked and the child object is automatically archived when removed from its parent (with no other backlinks).

No migration for non-file objects — only objects created after this feature ships are tracked.

---

## Changes

### 1. Set `createdInContext` on `BlockLinkCreateWithObject`

**File:** `core/block/create.go` — `CreateLinkToTheNewObject`

After the link block is created (so `linkID` is known), call `ModifyDetails` on the newly created object to set:
- `createdInContext = req.ContextId`
- `createdInContextRef = linkID`

No proto changes needed — `BlockLinkCreateWithObjectRequest` already carries `contextId`. The MW sets these fields automatically; clients don't need to change anything for this path.

For other creation paths (e.g. `ObjectCreate`), clients can still pass `createdInContext` explicitly in the `details` struct.

### 2. New `GCEligibleLayouts` whitelist

**File:** `core/domain/layout.go`

Add a new `GCEligibleLayouts` slice — the union of the existing `FileLayouts` plus user-content object types:

```go
var GCEligibleLayouts = []model.ObjectTypeLayout{
    // Files (existing FileLayouts)
    model.ObjectType_file,
    model.ObjectType_image,
    model.ObjectType_video,
    model.ObjectType_audio,
    model.ObjectType_pdf,
    // User content objects
    model.ObjectType_basic,
    model.ObjectType_profile,
    model.ObjectType_todo,
    model.ObjectType_set,
    model.ObjectType_note,
    model.ObjectType_bookmark,
    model.ObjectType_collection,
}
```

Excluded system layouts: `objectType`, `relation`, `relationOption`, `relationOptionsList`, `dashboard`, `space`, `spaceView`, `participant`, `date`, `chatDerived`, `discussion`.

### 3. Generalize GC queries

**File:** `core/files/filegc/filegc.go`

Replace every `domain.FileLayouts` filter in queries with `domain.GCEligibleLayouts`.

The early-return guard in `CheckFilesOnObjectArchived` ("if archived object is a file, return nil") is broadened: return early if the archived object's layout is **not in** `GCEligibleLayouts`. System objects can never be context parents for GC-tracked children.

All triggering entry points (`set_details.go`, `delete.go`, `smartblock.go`, `chats/service.go`) are unchanged.

Cascading works for free: when the GC archives child B, `triggerFileGCOnArchive` fires for B, which archives B's own children.

### 4. Deprecate `skipBin` in pages; keep only for chat

**File:** `core/block/editor/smartblock/smartblock.go`

The session-created link tracking (`sessionCreatedLinks`) and the `skipBin=true` path in `performFileGC` are removed. `CheckFilesOnLinksRemoval` is always called with `skipBin=false` from smartblock. This fixes an existing bug where `skipBin` was not working correctly for files in pages anyway.

`skipBin=true` is only ever passed from `chats/service.go` (chat message deletion). The per-layout override (Section 5 below) ensures non-file objects are never permanently deleted even from the chat path.

### 5. Per-layout `skipBin` override

**File:** `core/files/filegc/filegc.go`

Inside the GC, after resolving each candidate object's `resolvedLayout`, if the layout is not in `domain.FileLayouts`, force `skipBin = false` regardless of what the caller passed. This is a per-object decision inside the existing processing loop.

Files in chat messages are still permanently deleted as today. Non-file objects always go to Bin.

### 6. Rename `FileGC` → `ObjectGC`

Rename the `FileGC` interface to `ObjectGC` and the `fileGC` field in all dependents (`service.go`, `set_details.go`, `delete.go`, etc.). Package location (`core/files/filegc`) is unchanged to avoid import churn.

Method rename is optional — `CheckFilesOnLinksRemoval` etc. can be renamed to drop "Files" (e.g. `CheckOnLinksRemoval`) or left as-is.

---

## What is NOT changing

- **No migration** for non-file objects. Only objects created after this feature ships receive `createdInContext`.
- **File migration** (`core/migration/objectcontext.go`) is unchanged.
- No proto changes.
- No changes to client APIs beyond MW automatically setting `createdInContext` in `CreateLinkToTheNewObject`.

---

## Testing

New test cases in `filegc_test.go` (fixture pattern, matching existing style):

| Case | Expected |
|------|----------|
| Non-file object with `createdInContext`, parent archived, no other backlinks | Child archived |
| Non-file object with `createdInContext`, parent archived, other backlinks exist | Child kept |
| Non-file object, parent unarchived | Child restored |
| Non-file object, link removed, caller passes `skipBin=true` | Child archived (not deleted) |
| System-layout object with `createdInContext`, parent archived | Not GC'd |
| `CreateLinkToTheNewObject` call | Sets `createdInContext` and `createdInContextRef` on new object |
