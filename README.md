# joplin-axi

An [AXI](https://axi.md/)-style CLI for [Joplin](https://joplinapp.org/), talking directly to the Joplin Web Clipper (Data API). Built for AI agents rather than humans: no MCP schema overhead, [TOON](https://toonformat.dev)-formatted output, and structured errors/exit codes instead of interactive prompts.

Inspired by [joplin-mcp](https://github.com/alondmnt/joplin-mcp), a Python MCP server that exposes Joplin's Data API as 26 tool-call schemas. joplin-axi reimplements that same functional surface as a plain CLI instead of an MCP tool schema — see [TODO.md](TODO.md) for the tool-by-tool parity mapping. Narrow tools are deliberately collapsed into single flagged subcommands (e.g. `find_notes` + `find_notes_with_tag` + `find_notes_in_notebook` → one `notes list --query/--tag/--notebook`) rather than mirrored 1:1, per AXI's combined-operations principle.

## Setup

Not published to npm — install from GitHub.

```sh
git clone https://github.com/<your-username>/joplin-axi.git
cd joplin-axi
npm install
npm run build
npm link                                # puts `joplin-axi` on your PATH from this clone

export JOPLIN_TOKEN=<token from Joplin → Tools → Options → Web Clipper>
export JOPLIN_BASE_URL=http://localhost:41184   # default; override if needed
```

(Or copy `.env.example` to `.env` and load it however your shell/agent harness does.)

Skip the clone and `npm link` if you just want a one-off run:

```sh
npx github:<your-username>/joplin-axi ping
```

## Usage

```sh
joplin-axi                              # home view: connectivity, counts, recent notes
joplin-axi ping                         # connectivity + auth check

joplin-axi notebooks list [--parent <id>]     # pass --parent "" for top-level only
joplin-axi notebooks create <title> [--parent <id>] [--icon <emoji>]
joplin-axi notebooks update <id> [--title <text>] [--icon <emoji>] [--parent <id>]
joplin-axi notebooks delete <id>      # always a soft delete
joplin-axi notebooks restore <id>    # restores only this notebook, not trashed descendants

joplin-axi tags list
joplin-axi tags of <note-id>
joplin-axi tags create <title>
joplin-axi tags update <id> <title>
joplin-axi tags delete <id>           # tags have no trash concept — this is immediate
joplin-axi tags add <tag-title> (--notes <id[,id...]> | --query <text> | --notebook <id> | --task)
joplin-axi tags remove <tag-title> (--notes <id[,id...]> | --query <text> | --notebook <id> | --task)
# --notes and the filters are mutually exclusive; at least one selection method is required

joplin-axi notes list [--query <text>] [--notebook <id>] [--tag <title>] [--task] [--trash] [--limit <n>] [--fields <list>]
# --query/--notebook/--tag/--task all combine (intersected by note ID); --trash stands alone
joplin-axi notes get <id> [--full]
joplin-axi notes find-in <id> <pattern> [--ignore-case]
joplin-axi notes links <id>
joplin-axi notes resources <id>

joplin-axi notes create --title <text> [--body <text>] [--notebook <id>]
joplin-axi notes update <id> [--title <text>] [--body <text>] [--notebook <id>]
joplin-axi notes edit <id> [--find <text> --replace <text> [--all]] [--append <text>] [--prepend <text>]
joplin-axi notes delete <id>          # always a soft delete — moves to Joplin's trash, never permanent
joplin-axi notes restore <id>        # undoes it (clears deleted_time)

joplin-axi import <path> --notebook <id> [--format md|jex] [--on-duplicate skip|rename] [--dry-run]
# --notebook required for md (no default target); optional graft point for jex, which carries its own hierarchy
```

Every subcommand supports `--help` for its flags and examples.

## Development

```sh
npm run dev -- notes list          # run from source via tsx, no build step
npm run typecheck
npm test
npm run test:watch
```

## Design notes

- **Output format**: TOON tables/objects (`src/toon.ts`) — 3–4 default fields per list, `--fields` to expand, `--full` to bypass body truncation.
- **Errors**: usage errors exit `2` (unknown flag/command, missing argument), runtime errors exit `1`, success exits `0`. Errors print to stdout in the same structured format as normal output — see `errorOut()` in `src/toon.ts`.
- **No interactive prompts**: every operation is flag-driven.
- **Combined operations over 1:1 tool mirroring**: e.g. `notes list` merges what joplin-mcp splits into `find_notes` / `find_notes_with_tag` / `find_notes_in_notebook`.
- **`--query`/`--notebook`/`--tag`/`--task` all combine, via ID-based intersection, not query interpolation**: `--notebook`/`--tag` each fetch a *full* candidate set from their own ID-scoped endpoint, `--query`/`--task` fetch a capped candidate set from real full-text search (`--task` as a safe `type:todo` DSL suffix — safe because it's a fixed literal, not a user-controlled title), and when more than one is active, the result is their intersection by note ID. This sidesteps a real, empirically-confirmed injection risk in the alternative approach (resolving an ID to its title and interpolating that title into the search DSL as `notebook:"..."`) — see `runList` in `src/commands/notes.ts` and [TODO.md](TODO.md) for the live-reproduced silent-false-negative that ruled it out. The same set-intersection logic is shared (`src/lib/note-scope.ts`) with `tags add`/`tags remove`, which accept these filters as an alternative to explicit `--notes`.
- **Bulk mutations report, they don't gate**: `tags add`/`remove` with filters mutate immediately and print every affected note (id+title), rather than requiring a preview + `--yes` confirmation step. A confirm-gate would double round-trips even in the common correct case — against AXI's anti-friction design — and since tagging mistakes are cheap to reverse (one more `tags remove`/`add` call with the same filter), visibility-after gives the same practical safety without the tax.
- **`--trash`**: uses Joplin's `include_deleted=1`, which the [REST API docs](https://joplinapp.org/help/api/references/rest_api/) document only for the unfiltered `GET /notes` listing (not `/search`, `/folders/:id/notes`, or `/tags/:id/notes`) — so it can't combine with `--query`/`--task`/`--notebook`/`--tag`. Since Joplin mixes trashed notes into normal results rather than returning only them, `notes list --trash` over-fetches and filters client-side to approximate a trash listing.
- **`notes delete` is always a soft delete**: it calls plain `DELETE /notes/:id` and never sends `permanent=1`. There is deliberately no `--permanent` flag — see [TODO.md](TODO.md#phase-2--mutations-notes) for why.
- **`notes restore`/`notebooks restore`**: `PUT /notes/:id` (or `/folders/:id`) with `{"deleted_time": 0}` — undocumented in the [REST API reference](https://joplinapp.org/help/api/references/rest_api/) (deleted_time isn't listed among modifiable fields) but confirmed working: it's exactly what joplin-mcp's own `restore_from_trash` tool does under the hood (`tools/trash.py`, via the joppy client's generic `modify_note`/`modify_notebook`). An earlier pass here concluded this was unimplementable after checking the REST docs, the `joppy` client, and `joplin-mcp`'s `tools/notes.py` — it just hadn't looked at `tools/trash.py`, a separate file. Restoring a notebook only clears `deleted_time` on that one notebook; Joplin sets it on every descendant when a notebook is trashed, so sub-notebooks and notes inside stay trashed and must be restored individually.
- **Mutations are idempotent, checked live not assumed**: `tags add`/`remove` on a note already in the target state, and `notes`/`notebooks delete` on an already-trashed item, all exit `0` with no error — verified against real Joplin, not just inferred from the API docs (see [TODO.md](TODO.md) — "Code review pass").
- **`--fields` actually limits what's fetched, not just what's displayed**: `notes get` and `notes resources` only request the API fields needed for the requested output — e.g. `notes get <id> --fields id,title` never pulls the note body over the wire, and `notes resources` only fetches `ocr_text` (potentially large) when explicitly requested via `--fields`.
- **`--help` always wins over flag validation that comes after it** in the same command (e.g. `notes list --help --bogus` shows help, not an unknown-flag error) — but validation for anything *before* `--help` in the argv still applies, a deliberate trade-off to avoid misfiring on a flag value that happens to equal the literal string `--help`.
- **`import` supports markdown and JEX only**, not joplin-mcp's full 5-format surface (HTML/CSV/generic-file fallback are deferred — see [TODO.md](TODO.md#phase-5--import)). `tar` (`src/lib/import/jex-source.ts`) is joplin-axi's first and only runtime dependency — a hand-rolled tar parser was considered and rejected, since tar parsing is exactly the kind of edge-case-heavy, security-sensitive code (long filenames, checksums, path traversal) where a battle-tested library beats a bespoke one. A JEX archive is parsed **entirely in memory**, never extracted to disk — this both sidesteps tar-slip/path-traversal risk and fixes a real bug in the reference implementation, where extracted files get deleted (temp-directory cleanup) before the resource-upload pass runs, so JEX attachments there likely fail to embed in practice. JEX tag associations (Joplin stores tags as separate item types the reference never cross-references) are also fully reconstructed here, not just carried over from a literal `tags:` field that real exports don't set.
- **`import --notebook` is required for markdown, optional for JEX**: no default "Imported" notebook like the reference — an import target should be explicit, not assumed. `--dry-run` runs the parse phase only (zero Joplin API calls) and doesn't require `--notebook`, since it's meant for deciding on a target before committing to one.

## License

[MIT](LICENSE)
