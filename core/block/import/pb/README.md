# Anytype and AnyBlock import

Use the existing `ObjectImport` RPC with `type: Pb` and `pbParams.path` for:

- Legacy Anytype protobuf snapshots and protobuf JSON, directories, and ZIPs.
- Standalone AnyBlock v2 object JSON.
- AnyBlock v2 authored bundles and full space exports, as directories or ZIPs.
  ZIPs may wrap the bundle in one directory.

`github.com/anyproto/any-block/bundle/convert.Bundle` validates and converts the
bundle to native snapshots. Heart then uses its existing ID remapping and object
creation pipeline. Conversion covers stored property and option definitions,
built-in and custom types, templates, participant references, object links,
views, widgets, and space metadata. Manifest attachments are streamed from the
source filesystem and extracted only when the importer needs them.

Set `isNewSpace: true` when restoring into a newly created space to apply its
name, description, icon, homepage, and widgets. Otherwise the destination space
keeps its settings. The usual collection and experience options still apply.

Files without embedded bytes retain remote file metadata and encryption keys.
Availability still depends on access to the source network; conversion cannot
recover bytes absent from both the bundle and the network. Unknown property
definitions are not guessed; their values remain, with a warning. Uninstalled
property definitions are restored as live definitions so values remain editable.
Option identity hints follow the codec's liveness rules: in a new space they
fall back to names; duplicate option names can be ambiguous even though each
stored option definition is retained.

Standalone documents use the single-object codec; references needing bundle
definitions should be imported as a bundle. Validation failures abort conversion;
fidelity warnings are written to `import-anyblock` logs.

The shared converter currently lives in the sibling `any-block` checkout. The
local `go.mod` replacement must become a published dependency version before
building Heart without that checkout.

See [the sweep harness](../../../../cmd/anyblockimportsweep/README.md) for testing
retained per-space exports through a fresh headless account and the gRPC API.
