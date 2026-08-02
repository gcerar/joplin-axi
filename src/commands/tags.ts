import type { JoplinClient } from '../client.js';
import type { Command, CommandOutput } from '../lib/command.js';
import { type CommandSpec, type FlagSpec, type ParsedArgs, requirePositional, splitList, UsageError } from '../lib/args.js';
import { CHECK_SCOPES_HINT, resolveNoteScope } from '../lib/note-scope.js';
import { resolveTagId } from '../lib/tag-lookup.js';
import { help, object, sections, table } from '../toon.js';

// Shared by `tags add`/`tags remove`: pick notes either by an explicit --notes
// ID list, or by the same --query/--notebook/--task filters as `notes list`
// (intersected via resolveNoteScope — see src/lib/note-scope.ts). Deliberately
// no confirmation gate: mutating immediately and reporting every affected note
// is the AXI-consistent trade-off (a gate would double round-trips even in the
// common correct case; a wrong filter is visible immediately in the report and
// one more `tags remove` call away from being undone).
//
// An explicit --notes ID that doesn't resolve (typo, deleted note) is reported
// as a failure alongside any mutation failures rather than aborting the whole
// batch — one bad ID in a long --notes list shouldn't block tagging the rest.
async function resolveTargets(
  parsed: ParsedArgs,
  client: JoplinClient,
  usage: string
): Promise<{ targets: { id: string; title: string }[]; failed: { id: string; title: string; error: string }[] }> {
  const explicitNotes = parsed.flags.notes as string | undefined;
  const query = parsed.flags.query as string | undefined;
  const notebook = parsed.flags.notebook as string | undefined;
  const task = Boolean(parsed.flags.task);
  const hasFilters = Boolean(query || notebook || task);

  if (explicitNotes && hasFilters) {
    throw new UsageError('--notes cannot be combined with --query/--notebook/--task', [
      'use --notes for an explicit ID list, or filters to select notes by criteria — not both',
    ]);
  }
  if (!explicitNotes && !hasFilters) {
    throw new UsageError('requires --notes or at least one of --query/--notebook/--task', [usage]);
  }

  if (explicitNotes) {
    const ids = splitList(explicitNotes);
    const results = await Promise.allSettled(ids.map((id) => client.getNote(id, ['id', 'title'])));
    const targets: { id: string; title: string }[] = [];
    const failed: { id: string; title: string; error: string }[] = [];
    results.forEach((result, i) => {
      if (result.status === 'fulfilled') {
        targets.push({ id: result.value.id, title: result.value.title });
      } else {
        failed.push({ id: ids[i], title: '', error: result.reason instanceof Error ? result.reason.message : String(result.reason) });
      }
    });
    return { targets, failed };
  }

  const notes = await resolveNoteScope(client, { notebookId: notebook, query, task, fields: ['id', 'title', 'updated_time'] });
  return { targets: notes.map((n) => ({ id: n.id, title: n.title })), failed: [] };
}

// Applies `fn` to every target sequentially, collecting per-note failures
// instead of aborting on the first one — so a mid-batch error doesn't hide
// which notes were already mutated (the caller sees exactly what succeeded
// and what didn't, rather than a generic error with no report at all).
async function applyToEach(
  targets: { id: string; title: string }[],
  fn: (t: { id: string; title: string }) => Promise<void>
): Promise<{ succeeded: { id: string; title: string }[]; failed: { id: string; title: string; error: string }[] }> {
  const succeeded: { id: string; title: string }[] = [];
  const failed: { id: string; title: string; error: string }[] = [];
  for (const t of targets) {
    try {
      await fn(t);
      succeeded.push(t);
    } catch (e) {
      failed.push({ id: t.id, title: t.title, error: e instanceof Error ? e.message : String(e) });
    }
  }
  return { succeeded, failed };
}

function failedTable(failed: { id: string; title: string; error: string }[]): string {
  return failed.length ? table('failed', ['id', 'title', 'error'], failed) : '';
}

const SELECTION_FLAGS: Record<string, FlagSpec> = {
  notes: { type: 'string', description: 'Comma-separated note IDs (mutually exclusive with the filters below)' },
  query: { type: 'string', description: 'Only notes matching this full-text search' },
  notebook: { type: 'string', description: 'Only notes in this notebook ID' },
  task: { type: 'boolean', description: 'Only to-do notes', default: false },
};

const listSpec: CommandSpec = {
  name: 'tags list',
  summary: 'List all tags.',
  usage: 'joplin-axi tags list',
  flags: {},
  examples: ['joplin-axi tags list'],
};

async function runList(_parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const items = await client.listTags();
  const body = items.length ? table('tags', ['id', 'title'], items) : 'tags: 0 tags found';
  return sections(body, help(['Run `joplin-axi notes list --tag <title>` to see notes with a tag.']));
}

const ofSpec: CommandSpec = {
  name: 'tags of',
  summary: 'List tags applied to a note.',
  usage: 'joplin-axi tags of <note-id>',
  flags: {},
  examples: ['joplin-axi tags of 3f9c2a1b'],
};

async function runOf(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const noteId = requirePositional(parsed, 0, 'note-id', 'joplin-axi tags of <note-id>');

  const items = await client.getTagsByNote(noteId);
  const body = items.length ? table('tags', ['id', 'title'], items) : `tags: 0 tags on note ${noteId}`;
  return sections(body, help([`Run \`joplin-axi tags add <title> --notes ${noteId}\` to add a tag.`]));
}

// ── tags create ──────────────────────────────────────────────────────────────

const createSpec: CommandSpec = {
  name: 'tags create',
  summary: 'Create a new tag.',
  usage: 'joplin-axi tags create <title>',
  flags: {},
  examples: ['joplin-axi tags create urgent'],
};

async function runCreate(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const title = requirePositional(parsed, 0, 'title', 'joplin-axi tags create <title>');

  const tag = await client.createTag(title);
  return sections(
    object('tag', { id: tag.id, title: tag.title }),
    help([`Run \`joplin-axi tags add ${title} --notes <id[,id...]>\` to apply it.`])
  );
}

// ── tags update ──────────────────────────────────────────────────────────────

const updateSpec: CommandSpec = {
  name: 'tags update',
  summary: "Rename a tag.",
  usage: 'joplin-axi tags update <id> <title>',
  flags: {},
  examples: ['joplin-axi tags update 9876c25986874f4ea588fff1b3ff9c1b renamed-tag'],
};

async function runUpdate(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const [id, title] = parsed.positionals;
  if (!id || !title) throw new UsageError('requires <id> and <title>', ['joplin-axi tags update <id> <title>']);

  const tag = await client.updateTag(id, title);
  return sections(object('tag', { id: tag.id ?? id, title: tag.title }), help(['Run `joplin-axi tags list` to confirm the rename.']));
}

// ── tags delete ──────────────────────────────────────────────────────────────

const deleteSpec: CommandSpec = {
  name: 'tags delete',
  summary: "Delete a tag. Unlike notes/notebooks, Joplin has no trash for tags — this is immediate and only removes the tag and its note associations, not the notes themselves.",
  usage: 'joplin-axi tags delete <id>',
  flags: {},
  examples: ['joplin-axi tags delete 9876c25986874f4ea588fff1b3ff9c1b'],
};

async function runDelete(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi tags delete <id>');

  await client.deleteTag(id);
  return sections(object('tag', { id, deleted: true }), help(['Run `joplin-axi tags list` to confirm.']));
}

// ── tags add / remove ────────────────────────────────────────────────────────

const addSpec: CommandSpec = {
  name: 'tags add',
  summary: 'Apply a tag to notes — by explicit --notes IDs, or by --query/--notebook/--task filters (every match, intersected).',
  usage: 'joplin-axi tags add <tag-title> (--notes <id[,id...]> | --query <text> | --notebook <id> | --task)',
  flags: SELECTION_FLAGS,
  examples: [
    'joplin-axi tags add active --notes 3f9c2a1b',
    'joplin-axi tags add active --notes 3f9c2a1b,6f6a6757',
    'joplin-axi tags add active --notebook a1b2c3 --query "annual report"',
  ],
};

async function runAdd(parsed: ParsedArgs, client: JoplinClient): Promise<string | CommandOutput> {
  const title = requirePositional(parsed, 0, 'tag-title', addSpec.usage);

  const tagId = await resolveTagId(client, title);
  const { targets, failed: resolveFailed } = await resolveTargets(parsed, client, addSpec.usage);
  const { succeeded, failed: applyFailed } = await applyToEach(targets, (t) => client.addTagToNote(tagId, t.id));
  const failed = [...resolveFailed, ...applyFailed];

  const body = succeeded.length ? table('notes', ['id', 'title'], succeeded) : 'notes: 0 notes matched — nothing tagged';
  const failedBody = failedTable(failed);
  const hints = [
    succeeded.length ? `Run \`joplin-axi notes list --tag ${title}\` to see all notes with this tag.` : '',
    failed.length
      ? `${failed.length} note(s) failed — retry with \`joplin-axi tags add ${title} --notes <id[,id...]>\` for just those.`
      : '',
    !succeeded.length && !failed.length ? CHECK_SCOPES_HINT : '',
  ].filter(Boolean);

  const output = sections(object('tag', { title, added_to: succeeded.length, failed: failed.length }), body, failedBody, help(hints));
  return failed.length ? { output, exitCode: 1 } : output;
}

const removeSpec: CommandSpec = {
  name: 'tags remove',
  summary: 'Remove a tag from notes — by explicit --notes IDs, or by --query/--notebook/--task filters (every match, intersected).',
  usage: 'joplin-axi tags remove <tag-title> (--notes <id[,id...]> | --query <text> | --notebook <id> | --task)',
  flags: SELECTION_FLAGS,
  examples: ['joplin-axi tags remove active --notes 3f9c2a1b', 'joplin-axi tags remove active --notebook a1b2c3'],
};

async function runRemove(parsed: ParsedArgs, client: JoplinClient): Promise<string | CommandOutput> {
  const title = requirePositional(parsed, 0, 'tag-title', removeSpec.usage);

  const tagId = await resolveTagId(client, title);
  const { targets, failed: resolveFailed } = await resolveTargets(parsed, client, removeSpec.usage);
  const { succeeded, failed: applyFailed } = await applyToEach(targets, (t) => client.removeTagFromNote(tagId, t.id));
  const failed = [...resolveFailed, ...applyFailed];

  const body = succeeded.length ? table('notes', ['id', 'title'], succeeded) : 'notes: 0 notes matched — nothing removed';
  const failedBody = failedTable(failed);
  const hints = [
    succeeded.length ? `Run \`joplin-axi tags add ${title} --notes <id[,id...]>\` to undo, if needed.` : '',
    failed.length
      ? `${failed.length} note(s) failed — retry with \`joplin-axi tags remove ${title} --notes <id[,id...]>\` for just those.`
      : '',
    !succeeded.length && !failed.length ? CHECK_SCOPES_HINT : '',
  ].filter(Boolean);

  const output = sections(object('tag', { title, removed_from: succeeded.length, failed: failed.length }), body, failedBody, help(hints));
  return failed.length ? { output, exitCode: 1 } : output;
}

export const tagsCommands: Record<string, Command> = {
  list: { spec: listSpec, run: runList },
  of: { spec: ofSpec, run: runOf },
  create: { spec: createSpec, run: runCreate },
  update: { spec: updateSpec, run: runUpdate },
  delete: { spec: deleteSpec, run: runDelete },
  add: { spec: addSpec, run: runAdd },
  remove: { spec: removeSpec, run: runRemove },
};
