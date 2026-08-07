---
name: anytype
description: Read, search, create and edit Anytype objects through the anytype CLI. Use when the user asks to find notes/tasks/pages in Anytype, create objects, tick checkboxes, edit document text, fill tables, or reorganize content. Task-shaped verbs over the local API; results are numbered handles you pass back — never copy long ids.
---

# Anytype task tools

Twelve verbs over the local Anytype API. Everything composes through a
session: `find` numbers its results (1, 2, …) and sets the working space;
every other verb takes `--object <number>`.

Setup: the local Anytype app must be running; `ANYTYPE_API_KEY` holds an
API key from the app's settings (`ANYTYPE_API_URL` defaults to
`http://127.0.0.1:31009`).

## The loop

```sh
anytype spaces                             # space ids, when none is known
# Work — bafyspace1
anytype find --space bafyspace1 --type task --filter 'done = false'
# 1. Prepare the Q3 report (task)
# 2. Ship the beta (task)
anytype read --object 1 --mode outline     # block labels + structure
anytype edit-text --object 1 --block ab3f2 --find "Q3" --replace "Q4"
```

1. **spaces** when no space id is known — it lists `name — id`.
2. **find** next — it creates the handles and the working space.
3. **describe** before you create or set properties — property keys and
   select option names must match exactly; describe lists the live ones.
4. **read** before you edit blocks — block labels come from read
   (`--mode outline` for structure, full mode for text; table row and
   column labels come from full mode too).

## Intent → verb recipes

| Intent | Verb — not that other thing |
|---|---|
| complete/close a task object | `set-properties --object 1 --set '{"done":true}'` (or the status option describe shows, e.g. `{"status":"Done"}`) — NOT check-item, which is for checkbox blocks inside a document |
| tick a checklist line in a note | `check-item --object 1 --block ab3f2 --checked` |
| change one word/phrase | `edit-text` with a short unique snippet — never retype the block |
| delete a word/phrase | `edit-text --find "the phrase" --replace ""` — an empty replacement deletes |
| add notes/sections/checklists | `add-blocks --markdown '…'` — write markdown, the server parses it |
| fill one table cell | `set-cell` — never rewrite the table; row/col labels come from full read |
| clear one table cell | `set-cell … --value ""` — an empty value clears |
| assign to the current user | value `"@me"` — e.g. `--set '{"assignee":"@me"}'` |
| due dates | `today`, `tomorrow`, weekday names, `+3d`, or `2026-08-01` |
| find "my open tasks" | `--filter 'assignee = "@me" AND done = false'` |

## Filter strings

`--filter` is a compact expression, not JSON:
`done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)` ·
`status IN ("In progress", "Blocked")` · `name CONTAINS "report"` ·
`lastModifiedDate > daysAgo(7)`. String values take double quotes; date
presets are functions (`today()`, `currentWeek()`, `daysAgo(n)`).

## Caveats

- **Text is markdown source.** `edit-text` find/replace operates on the
  block's markup: `**`, `[`, `~~` in a replacement become real formatting.
  Escape with `\` when you mean the literal character.
- **Select options are never created by these verbs.** An unknown option
  name is an error listing the existing names — fix the spelling (option
  names are case-sensitive). `--create-missing` is the deliberate escape.
- **Handles expire on the next find.** Re-run `find` and use the new
  numbers; block labels survive per object. Every edit receipt names the
  object it changed (`ok — "Groceries": …`) — check it matches your intent.
- **One verb, one intent.** There is no batch; run verbs in sequence.
  Retries are safe: an identical re-run within a minute is deduplicated,
  including after a failed or timed-out attempt.
- Errors are self-describing and name valid alternatives — read them, fix
  the named field, retry once. Do not loop blindly.

## References

- `anytype tools` — the machine-readable manifest: per-tool JSON schema,
  worked example, GBNF grammar (constrained decoding), and the filter
  grammar (EBNF + GBNF). `--tier small` narrows it to the 8-tool set for
  ~8B models (`large`, the default, is all twelve).
- `anytype mcp --tier small|large` — serve the same tools over MCP stdio
  for LOCAL models (Ollama/LM Studio-class hosts). Coding agents reading
  this skill should keep using the verbs directly — the CLI is the
  intended delivery for them; `mcp` exists for hosts that cannot run
  commands. Session state is in-memory for the server's lifetime.
- `anytype help` — the verb list; `anytype <verb> --help` — its flags.
- Spec: `core/api/APIV2.md` §7 (the wrapper contract), `pkg/lib/anyblockjson/SPEC.md`
  (the document format the full-read mode returns).
