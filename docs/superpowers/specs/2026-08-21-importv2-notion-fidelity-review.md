# Notion import: what the warnings were hiding (2026-08-21)

A real workspace's import produced 960 issues. This is what reading all of them
against the API responses that caused them turned up, what changed, and what was
deliberately left alone.

Everything below is measured on the committed cassette
(`core/block/importv2/notion/testdata/cassettes/workspace.yaml`, 403 pages, 35 data
sources, 5248 blocks), reproducible with:

```
go test ./core/block/importv2/notion/ -run TestCassetteDiagnostic -v   # counts
go test ./core/block/importv2/notion/ -run TestRenderReport -v         # the report page
```

## 1. What the 960 issues actually said

Eleven distinct sentences, and the four biggest were 89% of the ledger:

| count | issue |
|-------|-------|
| 435 | `block type "unsupported" has no anytype counterpart` |
| 171 | `child "Untitled" could not be resolved` |
| 135 | `image/embed block has no accessible URL` |
| 129 | `property "X" of type "button"/"place" is not supported` |
| 67 | `property "X" references an unresolvable user` |
| 10 | `rollup "X" flattened to text` |

Every line began with a Notion id — a block id, a percent-encoded property id
(`%40egk`), or a page uuid — and the report printed one line per issue.

## 2. What Notion withholds, and how we now say so

**Buttons.** All 435 "unsupported" blocks are `{"unsupported":{"block_type":"button"}}`:
the API names the kind it refuses to expose. The old message was wrong twice (the
block is a button; the reason is Notion, not Anytype) and left 435
`Unsupported block (unsupported)` paragraphs in page bodies. Now reported per page
with a count, and no placeholder — there is no content to stand in for.

**Linked database views.** 171 of the 199 `child_database` blocks are linked views:
Notion returns the view block's own id (not fetchable as a database) and the title
"Untitled". They cost 171 doomed discovery probes, 171 unactionable warnings and 171
`Unresolved link: Untitled` paragraphs. Now recognised and reported as what they are.
`resolveChildByTitle` also refuses sentinel titles: "Untitled" is what Notion calls
every unnamed database AND every linked view, so title matching on it would link every
view in a workspace to one unrelated database.

**Files with no URL.** 73 of 78 images and 62 of 72 embeds come back with
`"url": ""`. Nothing was lost that we could have fetched; the message says so.

**People.** All user objects in this workspace arrive as `{"object":"user","id":…}`
with no name, because the integration's User Capabilities are set to "No user
information". The report now says how to fix it rather than reporting 67 anonymous
losses. Note for future work: fetching `/users` would NOT help — that endpoint 403s
under exactly this setting (which is why token validation probes `/users/me`).

## 3. What the importer was throwing away

| recovered | evidence |
|-----------|----------|
| 245 page/database icons, 66 callout icons | Notion's built-in icons are a name + colour; `applyIcon` knew only emoji and files. Named icons are type-only in Anytype, so pages and callouts take the nearest emoji (`notionIconEmoji`, keyed by the Anytype icon name so the two tables compose). The workspace goes from 2 objects with an icon to 247. |
| the notes inside 3 Notion AI transcripts (18 blocks) | A `transcription` block carries a title and hangs its notes underneath as ordinary children; `unsupported()` dropped them after the fetcher had paid for them. Any unknown block type now keeps its subtree. |
| 5 place values | Notion's location property carries a display name and an address; it was skipped as "not supported" while the text fits a plain text relation. |

## 4. Untitled pages

53 of 403 imported pages had no name. Cause: they are database rows whose title
property is empty — Notion shows them as "Untitled" too, so the import was faithful.
47 of them carry nothing at all (no title, no property value, no content, no icon),
and 45 of those are the blank filler rows of one Notion contact-list template.
Nothing in the workspace references them: no relation, no mention, no link.

Those 45 are now skipped, reported once per database. Rows that hold anything —
an icon, one property, any content — still import, untitled, as they are in Notion.
Only rows are eligible: a standalone empty page is somewhere its author made and can
find again.

The skip is reported per row (an info), because the engine's completeness invariant
requires an issue against every claimed key it never sees emitted.

## 5. Crawl cost

Synced-block originals were re-walked once per reference: 303 of 1286
block-children requests on this workspace, at Notion's ~3 requests a second. They are
now fetched once per run and handed out as copies (a hoisted subtree is re-ided in
place, so sharing the cached slice would corrupt the cache). Measured: 2036 requests
before, 1746 after, byte-identical output.

## 6. Left alone, deliberately

- **Empty media blocks emit no placeholder.** The block vanishes from the page and the
  report says why. Empty layout columns left behind are removed by the state
  normalizer (`removeEmptyLayoutBlocks`).
- **Rows that are untitled but not empty** (6 of the 53) import as they are.
- **Rollup flattening, verification properties, >25-item truncation**: no Anytype
  counterpart; reported, not worked around.
- **A dedicated issue code for "nothing to lose"**: Notion button properties report an
  info under `dataLoss` rather than earning a code of their own. The report groups by
  severity and message, so the vocabulary buys nothing.

## 7. Numbers, before → after

| | before | after |
|---|-------|-------|
| issues in the ledger | 960 | 818 |
| report lines | 954 | ~200 |
| placeholder paragraphs in page bodies | 606 | 0 |
| objects with an icon | 5 | 250 |
| nameless imported objects | 53 | 8 |
| API requests | 2036 | 1746 |
