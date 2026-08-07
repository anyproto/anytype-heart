# Synthetic workspace fixture format (importv2 LLM planner quality tests)

One fixture = one synthetic workspace = one JSON file. It is the planner's INPUT
(`[]schemaplan.ContainerSchema` after loading) plus machine-checkable EXPECTATIONS.

A fixture without expectations is not a test. Every fixture must assert something.

## File shape

```json
{
  "id": "corp-agency",
  "name": "Corporate — live events agency",
  "family": "corporate",
  "inspiration": "notion.com/templates/category/event-planning — 'Event Planning OS'",
  "notes": "What this workspace is, in two sentences. Then: which planner decisions it exercises.",
  "containers": [
    {
      "id": "db-projects",
      "name": "Client Projects",
      "properties": [
        {"id": "pStatus", "name": "Status", "format": "select",
         "options": ["Lead", "Scoping", "Contracted", "Live", "Wrapped"]},
        {"id": "pDue", "name": "Due Date", "format": "date"},
        {"id": "pOwner", "name": "Owner", "format": "multiSelect", "options": ["Ana", "Ben"]}
      ],
      "titles": ["Acme Q3 Summit", "Northwind Product Launch", "Vertex Sales Kickoff"]
    }
  ],
  "expect": {
    "sameKind":         [["db-a", "db-b"]],
    "differentKind":    [["db-a", "db-c"]],
    "bundled":          {"db-projects": {"pDue": "dueDate"}},
    "notBundled":       {"db-blog": ["pPublish"]},
    "sharedRelation":   [["db-a:pStatus", "db-b:pStatus"]],
    "separateRelation": [["db-a:pStatus", "db-c:pStatus"]]
  }
}
```

### Fields

- `id` — kebab-case, unique across fixtures, becomes the filename.
- `family` — exactly one of: `corporate`, `personal-family`, `mixed-personal-business`, `events`.
- `inspiration` — the real Notion template family it echoes. Be specific.
- `notes` — purpose, then the decision points exercised.
- `containers[].id` — stable, opaque, unique within the fixture. Use readable slugs
  (`db-projects`), NOT Notion UUIDs. The planner treats container ids as opaque.
- `containers[].properties[].id` — unique within its container only. Readable slugs.
- `containers[].titles` — 3–8 realistic page titles. Optional but strongly preferred;
  they become `ContainerSamples.Titles`. Omit for containers testing the no-samples path.

### `format` vocabulary — CLOSED SET, these exact strings

`text`, `select`, `multiSelect`, `date`, `number`, `checkbox`, `url`, `email`,
`phone`, `files`, `objects`

These match the planner prompt's own vocabulary one-for-one. Any other string is
invalid and the fixture will be rejected.

- `options` is REQUIRED for `select` and `multiSelect`, and MUST be absent for every
  other format. Option lists are 2–12 realistic values.
- `objects` means a relation to another database. Do not invent a target field —
  `ContainerSchema` carries no relation target today.

### Expectations

All ids are fixture-local. `db:prop` addresses one property. Every group takes TWO OR MORE
members, not just two — three duplicated databases are one `sameKind` group of three, and
their shared `Status` is one `sharedRelation` group of three. Groups are checked pairwise.

- `sameKind` — these containers MUST end up sharing one object type.
- `differentKind` — these containers MUST NOT share a type.
- `bundled` — this property MUST map to this bundled relation. Allowed values ONLY:
  `dueDate`, `done`, `tag`, `genre`, `email`, `phone`. Nothing else is a legal target.
- `notBundled` — this property MUST NOT be redirected to any bundled relation. This is
  how traps are asserted (a `Publish Date` that must stay "Publish Date", not become
  "Due date").
- `sharedRelation` — these two properties MUST land on the same relation.
- `separateRelation` — these two properties MUST stay distinct relations, even though
  they share a name.

Only state an expectation you are confident is the RIGHT product behaviour. If a case
is genuinely debatable, leave it out of `expect` and describe it in `notes` instead.
Over-asserting bakes taste into a regression suite.

### The coverage rule — `validate.py` enforces this

A `sameKind` group is only sound if EVERY member covers at least **0.5** of the group's
merged property set (the union of `(lowercased name, format)` pairs). Below that, merging
produces a type whose relations are mostly empty for that database's pages.

Calibrated against a real 37-database workspace: three duplicated databases scored 1.00 each
(the ideal merge), while a real LLM's merge of three task databases scored 0.14 / 0.52 / 0.48
— a 21-relation type on which one member filled 3 fields. That merge is the failure this rule
exists to catch. Run `python3 validate.py` before considering a fixture done.

## Decision points a good fixture set must cover

Each fixture should hit several; the set as a whole must hit all of them.

1. **Exact duplicate databases** — copies of one database (a template duplicated per
   client/quarter). Must collapse to ONE kind. → `sameKind`
2. **Same concept, disjoint schemas** — e.g. a garden-chores list (3 columns) and a
   software backlog (14 columns) both called "Tasks". Must NOT merge; a merged type
   would carry ~20 relations that each database fills a third of. → `differentKind`
3. **Same property name, different option vocabularies** — `Status` = Drafted/Posted
   in a content calendar vs Not started/In progress/Done in a project tracker. Must
   stay separate relations. → `separateRelation`
4. **Same property name, identical options** — `Priority` = High/Medium/Low in two
   sibling databases. Should share one relation. → `sharedRelation`
5. **Same property name, different formats** — `Owner` as `multiSelect` in one database
   and `text` in another. Must never share.  → `separateRelation`
6. **Bundled whitelist hits** — `Due Date`/`Deadline` (date), an email-format property,
   a phone-format property, a `Tags` multiSelect, a completion checkbox.
7. **Bundled traps** — date properties that must NOT become `dueDate`: `Publish Date`,
   `Created Date`, `Start Date`, `Last Contacted`, `Reported Date`. Checkboxes that are
   not completion: `Featured`, `Urgent`, `Pinned`. → `notBundled`
8. **Cross-domain collision** — one space holding personal and business databases that
   share property names (`Contacts` personal vs `Clients` business; `Budget` for a
   wedding vs for a client project). Must not fuse across domains.
9. **Scale variety** — at least one 2-property container and one 12+-property container.
10. **Messy real-world naming** — emoji in a property name, a trailing space, a
    "(SB)"-style suffix, a name that is a decorated version of a type word
    ("Recipe SB", "Meal Calendar DB"). These are drawn from a real workspace.

## Realism bar

These stand in for real user data. Generic filler ("Field 1", "Category A") fails the
purpose. Option values, titles and property names must read like an actual person's
Notion workspace: specific vendors, real-sounding names, plausible jargon per domain.

## Scale

6–14 containers per fixture. Enough to force grouping decisions; small enough that a
human can check the expectations by eye.
