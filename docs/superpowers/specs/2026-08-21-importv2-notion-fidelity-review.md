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
| 245 page/database icons, 66 callout icons (of Notion's whole 474-name icon set — see below) | Notion's built-in icons are a name + colour; `applyIcon` knew only emoji and files. Named icons are type-only in Anytype, so pages and callouts take the nearest emoji (`notionIconEmoji`, keyed by the Anytype icon name so the two tables compose). The workspace goes from 2 objects with an icon to 247. |
| the notes inside 3 Notion AI transcripts (18 blocks) | A `transcription` block carries a title and hangs its notes underneath as ordinary children; `unsupported()` dropped them after the fetcher had paid for them. Any unknown block type now keeps its subtree. |
| 5 place values | Notion's location property carries a display name and an address; it was skipped as "not supported" while the text fits a plain text relation. |

### The icon inventory

Notion publishes no list of its icon names: the API reference gives examples,
and the picker's catalogue lives in a JS chunk the login bundle never loads (I
went through the chunk manifest looking — 938 named chunks, none of them it).
Its icon FILES are public and named after the icon, though, so

    https://www.notion.so/icons/<name>_<color>.svg     200 = real, 404 = not

is a membership oracle. Candidates came from Anytype's own 390 icon names, the
Material Symbols (~3.6k) and Lucide (~1.8k) vocabularies, the names the recorded
workspace used, and the 201 `icon-*` chunks Notion's app ships; then two rounds
of variants (-alternate, -line, -filled, -outline, plurals, head words) until a
round returned nothing new. Yield: 454, then 18, then 0.

**474 names**, committed as `core/block/importv2/notion/testdata/notion-icons.txt`,
all of them mapped. `TestEveryNotionIconResolves` fails if one stops resolving or
reaches a page with no emoji, so a future refresh that finds new names shows up
as a failing test rather than as pages importing bare.

### Two channels, two vocabularies

A type carries a named icon (Anytype's 390); a page or callout can only carry an
emoji. Composing page emoji out of the icon mapping is lossy in one direction,
because emoji is the richer set: sixteen of Notion's food icons share the one
`nutrition` icon, all six chess pieces landed on `dice`, and church, mosque and
synagogue all became the same office block — 217 of the 476 names sat in a bucket
of three or more.

So pages consult `notionEmoji` first: 146 names that have a distinct emoji (🍌
banana, 🕌 mosque, ♟️ chess, 🪴 potted-plant, 🩸 blood-pressure). Everything else
composes as before, which keeps a page and its type agreeing wherever they can.
The two do diverge where the vocabularies do — a database with a banana icon gets
the `nutrition` icon, a page with the same icon gets 🍌 — and that is the point:
each channel says the most it can rather than both saying the least.

## 4. Untitled pages

53 of 403 imported pages had no name. Cause: they are database rows whose title
property is empty — Notion shows them as "Untitled" too, so the import was faithful.
47 of them carry nothing at all (no title, no property value, no content, no icon),
and 45 of those are the blank filler rows of one Notion contact-list template.
Nothing in the workspace references them: no relation, no mention, no link.

Those 45 now go to the BIN rather than the space, reported once per database. Rows
that hold anything — an icon, one property, any content — still import, untitled, as
they are in Notion. Only rows are eligible: a standalone empty page is somewhere its
author made and can find again.

The bin rather than a skip, and this is the load-bearing part: a row is not
free-standing. Its database lists it as a member and another row's relation may point
at it, and both of those are written before any row is fetched — the collection is
emitted when the database converts, which happens before its rows are read. Dropping
the objects would turn each of those references into a dangling one, and the resolver
reports every one of them ("reference target was not part of the import"): 45 pieces
of clutter traded for 45 warnings. Archived, the rows keep their references, stay out
of every view, and can be restored by anyone who disagrees.

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

## 7. What the review pass changed after the fact

An adversarial review of the diff (five dimensions, every finding re-checked by a
second agent trying to refute it) confirmed eight things. Two were mine and serious:

- **A package-global map counting block kinds** had been left in `mapBlock` by an
  investigation. Imports are not serialised — a resumed crawl and a user-started
  import run at once — so two Notion runs would write it concurrently. The verifier
  reproduced `fatal error: concurrent map writes`, which no `recover()` can catch.
- **Grouping by message text** meant the markdown converter, whose messages still
  interpolated file names, would produce one summary row per issue: a vault with 300
  broken references measured at a 301-row table. Its messages were migrated, and the
  table now folds everything past 40 kinds into one counted line, so an unusual run
  degrades instead of exploding.

The rest: occurrences labelled as objects, an occurrence count called a line count,
names remembered at emission rather than at persist (which would mention-link objects
whose persist then failed), type suggestions interpolating type and reason into the
message, and three values in the emoji table that are not emoji.

Also caught by reading the cache code: the synced-original cache was keyed by block id
alone, but a walk's answer depends on the depth budget it ran under — a subtree first
fetched near the depth guard comes back cut short, and every shallower reference would
have reused the truncated copy.

## 8. Numbers, before → after

| | before | after |
|---|-------|-------|
| issues in the ledger | 960 | 396 rows, 818 occurrences |
| report lines | 954 | ~200 |
| placeholder paragraphs in page bodies | 606 | 0 |
| objects with an icon | 5 | 250 |
| nameless objects in the space | 53 | 8 (45 in the bin) |
| API requests | 2036 | 1746 |
