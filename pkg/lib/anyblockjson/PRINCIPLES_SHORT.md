# AnyBlock JSON — the rules, on one screen

The one-screen version of `PRINCIPLES.md`, which carries the rationale and
the evidence. `SPEC.md` is the format itself.

AnyBlock JSON is the one document shape for an Anytype object: what export
writes, what import reads, what a bundle ships, what API v2 serves and
accepts. It is written as often by a language model as by a person, and it
must be readable by someone who has never seen Anytype's internals.

## Ten rules

1. **Lossless for meaning.** What the user expressed survives a round trip;
   what the system bookkeeps may be normalized; every accepted loss is
   written down.
2. **Readable by a stranger.** A person who has never seen Anytype internals
   can read a document, understand it, and hand-edit it.
3. **Borrow words, don't coin them.** HTML, CommonMark, SQL, our own public
   API, the vocabulary block editors share; six Anytype terms; no name the
   format or the bundle mints says `relation` — only recorded stored keys
   and user-given names still do.
4. **Nothing to guess.** A valid document needs only what is in the author's
   head and in one example — no offsets, no ids to fetch first, no
   bookkeeping. Small models included.
5. **Token-efficient, not terse.** Spend no token that carries no meaning;
   never save one by making the reader decode.
6. **Names, not ids.** Wherever a human would write a name, the format
   carries the name.
7. **A document stands alone.** One exported object is understandable and
   re-importable without the space it came from.
8. **Strict in, canonical out.** One spelling on export; strict,
   path-addressed validation on import; never guess — fail loudly.
9. **One shape, every door.** Export, import, bundles, API v2, templates,
   prompts: the same document, no dialects.
10. **Evolution is explicit.** One version integer; a reader refuses what it
    does not know; every change is a bump.

## When rules collide

The user's meaning survives › a model (a small one too) can write it › a
stranger can read it › it costs few tokens › it is convenient to implement
or faithful to an internal name.

Two things never yield: `Marshal` never emits what `Validate` rejects, and
no loss is silent.

## Not

A sync or wire format · a replacement for the Markdown export · byte-equal
to arbitrary input · forward-compatible.

## Using the rules

A change to the format, to `pkg/lib/anyblockjson`, or to API v2's document
surface names the rule it serves — or the rule it bends and the cost it
accepts. Rules move on evidence, never on taste.
