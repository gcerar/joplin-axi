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
- **`notes delete` is always a soft delete**: it calls plain `DELETE /notes/:id` and never sends `permanent=1`. There is deliberately no `--permanent` flag — see [TODO.md](TODO.md#phase-2--mutations-notes) for why (and why `notes restore` doesn't exist yet: no restore/undelete mechanism could be found in the REST API docs, the `joppy` client, or `joplin-mcp`'s own source).
- **Mutations are idempotent, checked live not assumed**: `tags add`/`remove` on a note already in the target state, and `notes`/`notebooks delete` on an already-trashed item, all exit `0` with no error — verified against real Joplin, not just inferred from the API docs (see [TODO.md](TODO.md) — "Code review pass").
- **`--fields` actually limits what's fetched, not just what's displayed**: `notes get` and `notes resources` only request the API fields needed for the requested output — e.g. `notes get <id> --fields id,title` never pulls the note body over the wire, and `notes resources` only fetches `ocr_text` (potentially large) when explicitly requested via `--fields`.
- **`--help` always wins over flag validation that comes after it** in the same command (e.g. `notes list --help --bogus` shows help, not an unknown-flag error) — but validation for anything *before* `--help` in the argv still applies, a deliberate trade-off to avoid misfiring on a flag value that happens to equal the literal string `--help`.

## License

[MIT](LICENSE)
