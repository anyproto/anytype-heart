# AnyBlock JSON switched to raw display names — what the API layer needs to know

Written for the API v2 workstream, which merged `go-7383-anyblockjson` at
`1ccf34e7c` — one commit before the switch. Nineteen commits landed after that
point. **`core/api` was deliberately not touched**: `git diff --stat
1ccf34e7c..HEAD -- core/api/` is empty. Nothing here has broken the API. But the
format now spells properties differently from the API, and that divergence is a
decision the API layer owns rather than one this change made for it.

---

## 1. What changed

The format used to spell a property key by deriving an api slug — the stored key
run through `strcase.ToSnake`, or the space's `apiObjectKey` when it had one:

```json
"properties": { "name": "Sprint 12", "created_date": "…", "due_date": "…" }
```

It now spells every property and type key by the entity's **display name**, NFC
normalized and otherwise verbatim — bundled entities included, from the
`relations.json` / `types.json` names:

```json
"properties": { "Name": "Sprint 12", "Creation date": "…", "Due date": "…" }
```

`api_object_key` is no longer read anywhere on the format's resolution path. It
is still written by `objectcreator` and still read by `core/api` — that half is
untouched.

## 2. Why

Four things drove it, in the order they were established.

**A measured eval, not a preference.** The concern was that small models fumble
space-bearing JSON keys. Tested A/B on `google/gemma-4-e4b` (192 generations,
208 property observations per arm), graded by the real codec. Result: raw names
were handled *at least as well* as snake_case — 152/160 vs 152/160 on the
classes where both conventions have an unambiguous target, p = 1.0, and 192/192
parsed as JSON with zero dropped spaces, zero case drift, zero invisible-character
keys. The failures ran the other way: asked to *derive* a key, models improvised
(`완료` → `completed`, `Дата выполнения` → `due_date`) and improvised
*differently across documents* — cross-document key stability 85.7% for derived
against 96.6% for raw.

**Derivation was a step in every writer's path.** Normalization is computable
from the name, but every model has to do it, every time, and get it right. Raw
naming deletes the step.

**It deleted a whole failure class.** Under normalization a name could fail to
produce a key at all — `#`, `☕`, `C++` normalize to empty or to `c` — so the
format carried an empty-normalization fallback, a leading-`_` escape for
digit-initial and keyword names (`50% done` → `_50_done`), and a stored-key
fallback. None of those can arise now; all three are deleted rather than
reimplemented.

**Transliteration was actively wrong.** `unidecode` rendered Japanese 作業内容 as
the Chinese reading `zuo_ye_nei_rong`, Korean 완료 as `wanryo`, and Arabic مهمة
as `mhm@` — with an `@` the api key grammar does not even admit. The format's own
rule preserves the script: `Задача` stays `задача`, `作業内容` stays `作業内容`.

## 3. Resolution is forgiving, and that matters for the API's choice

The format folds on read — casefold plus separator-strip — so **the old
spellings still resolve**:

```
"Name"  "name"  "NAME"                          → name
"Creation date"  "creation_date"  "created_date" → createdDate
"Due date"  "due_date"  "DUE DATE"               → dueDate
```

An API that keeps emitting `due_date` into an AnyBlock document is understood.
The reverse is not automatic: a consumer that expects `due_date` and receives
`"Due date"` needs the same fold, which `bundle.FoldApiKey` provides.

## 4. What did change under the API's feet

`core/api` is untouched and its **api keys are unchanged** — `bundle.ApiSlug`
derives from the internal *key*, never from the name, so `audio_genre` and
`space` remain what they were. `ApiSlug`, `ApiSlugFromName`, `SanitizeApiSlug`,
`MintApiSlug`, `MintApiSlugFromName` and `FoldApiKey` are all still there.

**But thirteen bundled display NAMES moved.** If anything in the API matches on
a name rather than a key, check these:

| stored key | was | now |
|---|---|---|
| `relationKey` | Relation key | **Property key** |
| `relationOptionColor` | Relation option color | **Property option color** |
| `relationReadonlyValue` | Relation value is readonly | **Property value is readonly** |
| `relationFormatObjectTypes` | Relation's target object types | **Property's target object types** |
| `featuredRelations` | Featured Relations | **Featured properties** |
| `headerRelationsLayout` | Header relations layout | **Header properties layout** |
| `recommendedRelations` | Recommended relations | **Recommended properties** |
| `recommendedFeaturedRelations` | Recommended featured relations | **Recommended featured properties** |
| `recommendedHiddenRelations` | Recommended hidden relations | **Recommended hidden properties** |
| `recommendedFileRelations` | Recommended file relations | **Recommended file properties** |
| type `relationOption` | Relation option | **Property option** |
| `audioGenre` | Genre | **Audio genre** |
| type `space` | Space | **Space settings** |

All ten `relation*` relations are `hidden: true`, so no user-visible label moved
for them. `audioGenre` and the `space` type are the two user-visible changes —
`space` is hidden too, and `spaceView` deliberately keeps the name "Space",
because that is the object a user thinks of as a space.

The first eleven exist because the retired alias table (`alias.go`) used to respell
`relationKey` as `property_key` on the wire. Deleting that table without renaming
the underlying names would have put "Relation" back into the format on ~6,900
documents. The rename does the same job with no table behind it.

## 5. The decision the API layer owns

The format is settled. The API surface is not, and these are genuinely separate
— the format is a document at rest, the API is a request/response contract with
different consumers and a different compatibility story.

**Option A — keep `api_key`.** No client breaks, and a stable identifier that
survives renames is exactly what a long-lived integration wants. Cost: two
vocabularies in one product, and a caller who reads a bundle and then calls the
API has to translate between them.

**Option B — switch to raw names.** One vocabulary everywhere; an agent reading a
user's "set Due Date to Friday" maps it straight through. Cost: a breaking change
for existing clients, and renames move the address.

**Option C — configurable, stated explicitly per request.** A header or parameter
declaring which vocabulary the caller speaks, so both are first-class and the
choice is the caller's. Cost: two code paths to keep honest, and a default to
choose for callers who say nothing.

**A shape worth considering** — it follows from the audiences rather than from
the format:

- **Tool wrappers and agent-facing surfaces: raw names.** The whole argument for
  the format applies unchanged. An agent has the user's words and the document's
  words; making them the same removes a translation step that the eval showed
  models perform unreliably. This is the case where raw names are clearly right.
- **Programmatic integrations (CRM-style, long-lived scripts): `api_key`.** A
  stable identifier that survives a rename is the point. A CRM sync addressing
  `?tags[in]=urgent` should not break because someone retitled a property in the
  UI.

That is Option C with the default decided by surface rather than by caller — the
tool layer speaks names, the raw REST surface speaks keys, and each is the right
default for who is holding it.

**One consequence to weigh if the API keeps `api_key`:** 22 of 514 option api
keys in a 77-space account are *not* derivable from the current name — they were
minted before a rename and kept the old spelling (`"Awareness"` → `discovery`,
`"Chat Management"` → `chat_managemetn`, typo included). So `api_key` is not a
pure function of the name, and any code assuming it can re-derive one is wrong on
about 4% of real options. That cuts both ways: it is the strongest argument that
`api_key` carries real information a name does not, and the strongest argument
that it is a second identity to keep in sync.

## 6. Things that are useful whichever way you go

- **`bundle.FoldApiKey`** — casefold + strip `_`/`-`. Exact match should always
  win before the fold is consulted; two keys folding together is an ambiguity to
  surface loudly, never to resolve by guess.
- **`MintApiSlug` / `MintApiSlugFromName`** (`pkg/lib/bundle/apislug.go`) — the
  grammar-safe minting helpers. Added because the app was storing api keys that
  no key route could accept: measured, 27 of 1,530 stored keys violated
  `^[A-Za-z0-9_]+$`, including `lists_[in_work]`, `manual_export_&_import` and
  `[?]_medium` — the last being `➡️ Medium` with the emoji unidecoded to a
  literal `[?]`. All six app minting sites now route through these.
- **The API's key derivation is unchanged** and still splits on case and digit
  boundaries (`Web3` → `web_3`, `P2P` → `p_2_p`). The format deliberately does
  *not* — it has no derivation step at all: a name IS the key, NFC-normalized
  and otherwise verbatim. If the API ever adopts name-derived keys, that
  difference matters.
- **The compact filter grammar has no quoted-key form**, so a spelling no
  identifier folds onto — `C++`, `50% done` — has no compact representation.
  Recorded in SPEC §6.2.1. Relevant if the API exposes compact filters over
  name-spelled properties.

## 7. Where to read more

- `pkg/lib/anyblockjson/SPEC.md` §3 — the authority on spelling and resolution.
- `pkg/lib/bundle/apislug.go` — the API's own slug machinery, unchanged.
