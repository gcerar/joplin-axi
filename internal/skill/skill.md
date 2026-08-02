---
name: joplin-axi
description: "Operate this Joplin instance through the joplin-axi CLI — search and browse notes by text/notebook/tag/to-do status, read note content and attachments, create/edit/move/delete notes, and manage notebooks and tags. Use whenever a task touches this Joplin instance: searching or listing notes, reading or editing note content, filing notes into notebooks, tagging/untagging, or managing notebooks/tags."
user-invocable: false
author: Gregor Cerar
metadata:
  hermes:
    tags: [joplin, notes, notebooks, tags, knowledge-base]
    category: productivity
---

# joplin-axi

Agent-ergonomic CLI for a single Joplin instance's Web Clipper (Data) API, built on the [AXI](https://axi.md/) design principles — TOON output, minimal schemas, structured errors — instead of MCP.

joplin-axi requires `JOPLIN_TOKEN` (from Joplin → Tools → Options → Web Clipper) set in the environment, and `JOPLIN_BASE_URL` if Joplin isn't reachable at the default `http://localhost:41184`. See the project's README for installation.

## When to use

Use joplin-axi whenever a task touches this Joplin instance: searching or browsing notes, reading a note's full content or attachments, creating/editing/moving/deleting notes, or managing notebooks and tags.

## Workflow

1. Run `joplin-axi` with no arguments for a live snapshot: connectivity, notebook/tag counts, and the 5 most recently updated notes.
2. Browse or search: `notes list --query <text>`, `notes list --notebook <id>`, `notes list --tag <title>`, `notes list --task` (see Tips for which of these combine).
3. Drill into one note: `notes get <id>` (truncated body by default, `--full` for the whole thing), `notes find-in <id> <pattern>` for a regex search inside it, `notes links <id>` for outgoing links, `notes resources <id>` for attachments.
4. Mutate notes with `notes create`, `notes update`, `notes edit` (find/replace, append, prepend), `notes delete`/`notes restore`.
5. Manage structure with `notebooks list/create/update/delete/restore` and `tags list/of/create/update/delete/add/remove`.
6. Bring in external notes with `import <path> --notebook <id>` (markdown file/directory or a Joplin `.jex` export) — run `--dry-run` first to preview counts before writing anything.
7. Every response ends with contextual `help:` next-step hints — follow them.

## Commands

```
commands[3]:
  notes=list,get,find-in,links,resources,create,update,edit,delete,restore
  notebooks=list,create,update,delete,restore
  tags=list,of,create,update,delete,add,remove
```

Plus top-level `ping` (connectivity/auth check), `import <path>` (see Tips), and the no-args home view. Run `joplin-axi <group> <command> --help` or `joplin-axi import --help` for a command's flags and examples.

## Tips

- Output is TOON-encoded and token-efficient, with structured errors (`error:`/`help:` lines) and exit codes 0/1/2 (success/runtime error/usage error) — not JSON.
- `notes delete` and `notebooks delete` are **always soft deletes** (moved to Joplin's trash) — there is no `--permanent` flag, by design. `notes restore <id>`/`notebooks restore <id>` undo it (clears `deleted_time` via `PUT`, undocumented in the REST API reference but confirmed working). Restoring a notebook restores only that notebook — sub-notebooks and the notes inside it stay trashed and must be restored individually (`notes list --trash` to find them).
- `tags delete` has no trash concept at all in Joplin — it's immediate, and only removes the tag/its note associations, never the notes themselves.
- `--query` performs Joplin's real full-text search (SQLite FTS4, word-tokenized — e.g. `cat` won't match `cataclysmic`), across note titles and bodies, including to-do notes.
- `--query`, `--notebook`, `--tag`, and `--task` all combine freely on `notes list` — each active filter contributes a candidate set (notebook/tag: full, from their own ID-scoped endpoint; query/task: capped, from real search) and the result is their intersection by note ID. No notebook/tag title ever gets folded into a search query string.
- A very broad `--query` combined with a narrow `--notebook`/`--tag` could in principle miss a match beyond the search-side fetch cap (`max(limit*20, 500)`) — not observed in practice, but worth knowing if a combined search looks suspiciously empty.
- `--trash` on `notes list` only works alone (not combinable with `--query`/`--task`/`--notebook`/`--tag`) — Joplin's `include_deleted` is documented only for the unfiltered listing.
- `--notebook` takes a notebook ID (from `notebooks list`); `--tag` takes a tag *title* (from `tags list`), resolved to an ID internally.
- `notebooks create`/`update --icon <emoji>` takes a single emoji; joplin-axi encodes it into Joplin's internal icon format for you.
- `notebooks list --parent <id>` shows only that notebook's direct children; pass an empty string (`--parent ""`) to list only top-level notebooks.
- `tags add`/`tags remove` accept the same `--query`/`--notebook`/`--task` filters as `notes list` (mutually exclusive with explicit `--notes <id[,id...]>`; one of the two is required) to select notes by criteria instead of by ID. They mutate immediately and print every affected note (id+title) — no confirm-gate. If a filter matches more than expected, the fix is a follow-up `tags remove`/`add` call with the same filter, not a prompt.
- A note that fails in `tags add`/`remove` (unresolvable `--notes` ID, or the tag/untag call itself failing) doesn't abort the rest of the batch — it's listed in a separate `failed` table with its error, alongside the notes that succeeded. The command exits `1` if *any* note failed, even though it still prints a normal report (not the `error:`/`help:` format) and the successful notes are already mutated — check the `failed` count/table, not just the exit code, to know what actually needs retrying.
- `notes get`/`notes resources --fields <list>` restricts what's actually fetched from Joplin, not just what's displayed — asking for fewer fields (e.g. `notes get <id> --fields id,title`) skips pulling the note body over the wire at all.
- `notes resources --fields ...,ocr_text` truncates long OCR text to a preview by default, with a `--full` flag to see the complete text — same pattern as `notes get`'s body.
- All mutations here are idempotent: re-tagging an already-tagged note, untagging an already-untagged one, or deleting an already-trashed note/notebook all succeed silently (exit 0) rather than erroring.
- `--help` always wins over flag validation for anything *after* it in the same command (`notes list --help --bogus` shows help); validation for tokens *before* `--help` still applies.
- `notes edit --replace <text>` requires `--find`, and `--all` only applies with `--find`/`--replace` — both error explicitly rather than being silently ignored if passed alongside `--append`/`--prepend`.
- `notes edit --find/--replace` treats the replacement text literally — `--replace` values containing `$&`, `$$`, `` $` ``, or `$'` are inserted verbatim, not interpreted as JS-style replacement patterns.
- `--limit` (and any numeric flag) rejects a non-numeric value with a usage error rather than silently becoming `NaN`; `notes list`/`notes find-in` also reject zero or negative `--limit` explicitly.
- `notes list`'s zero-result message names every active scope (`--query`/`--notebook`/`--tag`), not just the query text.
- `notes find-in` and `notes links` end with a next-step hint when there's something actionable to suggest: `find-in` points at `notes get <id> --full` (and flags if any context line was truncated); `links` suggests viewing a linked note, but only if at least one *internal* link was found — an empty or all-external link list gets no hint.
- `import <path>` supports **markdown** (a single `.md` file or a directory — subdirectories become nested notebooks, frontmatter `title`/`tags`/`notebook`/`created`/`updated`/`is_todo` override the derived values) and **JEX** (`.jex`, Joplin's own export format — notebook/tag/note structure is reconstructed from the archive itself, including tag associations and attachments).
- `import --notebook <id>` is **required for markdown** (no default target — always name one explicitly) but **optional for JEX**, which carries its own notebook hierarchy; give it to graft that hierarchy under an existing notebook instead of recreating it top-level.
- `import --dry-run` parses the source and reports counts (notebooks/tags/notes/resources) without writing anything to Joplin — no `--notebook` needed for this. Run it first on anything unfamiliar.
- `import --on-duplicate` controls what happens when an imported note's title already exists in the target notebook: `skip` (default, nothing created) or `rename` (appends ` (1)`, ` (2)`, ... — also applied against duplicate titles within the same import batch, not just pre-existing ones).
- `import`'s note-to-note and note-to-resource links (both JEX `:/id` tokens and markdown relative `.md` links) are rewritten to point at the newly-created IDs once the whole batch exists — a link to something outside the current import batch (e.g. a note that already existed in Joplin before) is left untouched, not treated as broken.
- Markdown import also uploads **local file references** (`![diagram](./diagram.png)`, a relative link to a real sibling file that isn't another imported note) as Joplin resources — the same as JEX attachments get. A link to a file that doesn't actually exist on disk is left exactly as written, not flagged.
- A failed note in `import` doesn't abort the batch — everything else still gets created; check `notes_failed`/`notes_skipped`/`unresolved_links` in the report, not just the exit code (which is `1` if anything failed).
