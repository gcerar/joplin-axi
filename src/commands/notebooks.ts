import type { JoplinClient } from '../client.js';
import type { Command } from '../lib/command.js';
import { type CommandSpec, type ParsedArgs, requirePositional, UsageError } from '../lib/args.js';
import { help, object, sections, table } from '../toon.js';

// Format joplin-mcp itself uses for the Folder.icon field (undocumented in the
// REST API reference beyond "text") — {type:1, emoji, name:""}.
function encodeIcon(emoji: string): string {
  return JSON.stringify({ type: 1, emoji, name: '' });
}

const listSpec: CommandSpec = {
  name: 'notebooks list',
  summary: 'List all notebooks.',
  usage: 'joplin-axi notebooks list [--parent <id>]',
  flags: {
    parent: {
      type: 'string',
      description: 'Only notebooks directly under this parent ID (pass an empty string for top-level only)',
    },
  },
  examples: [
    'joplin-axi notebooks list',
    'joplin-axi notebooks list --parent c8a068acf54642a9b50f7f5a45195e2a',
    'joplin-axi notebooks list --parent ""',
  ],
};

async function runList(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const parent = parsed.flags.parent as string | undefined;
  const allItems = await client.listNotebooks();
  const items = parent === undefined ? allItems : allItems.filter((n) => n.parent_id === parent);

  const body = items.length
    ? table('notebooks', ['id', 'title', 'parent_id'], items)
    : `notebooks: 0 notebooks found${parent !== undefined ? ` under parent \`${parent || '(top-level)'}\`` : ''}`;

  return sections(body, help(['Run `joplin-axi notes list --notebook <id>` to see notes in a notebook.']));
}

// ── notebooks create ─────────────────────────────────────────────────────────

const createSpec: CommandSpec = {
  name: 'notebooks create',
  summary: 'Create a new notebook.',
  usage: 'joplin-axi notebooks create <title> [--parent <id>] [--icon <emoji>]',
  flags: {
    parent: { type: 'string', description: 'Parent notebook ID (omit for top-level)' },
    icon: { type: 'string', description: 'Emoji icon for the notebook' },
  },
  examples: ['joplin-axi notebooks create "Side project" --icon 🚀', 'joplin-axi notebooks create "Sub notebook" --parent c8a068acf54642a9b50f7f5a45195e2a'],
};

async function runCreate(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const title = requirePositional(parsed, 0, 'title', 'joplin-axi notebooks create <title> [--parent <id>] [--icon <emoji>]');

  const fields: Record<string, unknown> = { title };
  if (parsed.flags.parent !== undefined) fields.parent_id = parsed.flags.parent;
  if (parsed.flags.icon !== undefined) fields.icon = encodeIcon(String(parsed.flags.icon));

  const notebook = await client.createNotebook(fields);
  return sections(
    object('notebook', { id: notebook.id, title: notebook.title }),
    help([`Run \`joplin-axi notes list --notebook ${notebook.id}\` to see notes in it.`])
  );
}

// ── notebooks update ─────────────────────────────────────────────────────────

const updateSpec: CommandSpec = {
  name: 'notebooks update',
  summary: 'Rename, re-icon, or move a notebook under another parent.',
  usage: 'joplin-axi notebooks update <id> [--title <text>] [--icon <emoji>] [--parent <id>]',
  flags: {
    title: { type: 'string', description: 'New title' },
    icon: { type: 'string', description: 'New emoji icon' },
    parent: { type: 'string', description: 'Move under this parent notebook ID' },
  },
  examples: ['joplin-axi notebooks update c8a068acf54642a9b50f7f5a45195e2a --title "Renamed"'],
};

async function runUpdate(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notebooks update <id> [--title] [--icon] [--parent]');

  const fields: Record<string, unknown> = {};
  if (parsed.flags.title !== undefined) fields.title = parsed.flags.title;
  if (parsed.flags.icon !== undefined) fields.icon = encodeIcon(String(parsed.flags.icon));
  if (parsed.flags.parent !== undefined) fields.parent_id = parsed.flags.parent;

  if (Object.keys(fields).length === 0) {
    throw new UsageError('nothing to update — pass at least one of --title/--icon/--parent', [
      'joplin-axi notebooks update <id> [--title] [--icon] [--parent]',
    ]);
  }

  const notebook = await client.updateNotebook(id, fields);
  return sections(
    object('notebook', { id: notebook.id ?? id, title: notebook.title, updated: 'ok' }),
    help([`Run \`joplin-axi notebooks list\` to confirm the change.`])
  );
}

// ── notebooks delete ─────────────────────────────────────────────────────────

const deleteSpec: CommandSpec = {
  name: 'notebooks delete',
  summary: "Move a notebook (and its notes) to Joplin's trash. Always a soft delete — joplin-axi never permanently deletes.",
  usage: 'joplin-axi notebooks delete <id>',
  flags: {},
  examples: ['joplin-axi notebooks delete c8a068acf54642a9b50f7f5a45195e2a'],
};

async function runDelete(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notebooks delete <id>');

  await client.deleteNotebook(id);
  return sections(object('notebook', { id, trashed: true }), help([`Run \`joplin-axi notebooks restore ${id}\` to undo.`]));
}

// ── notebooks restore ────────────────────────────────────────────────────────

const restoreSpec: CommandSpec = {
  name: 'notebooks restore',
  summary:
    "Restore a notebook from Joplin's trash. Only restores this one notebook — sub-notebooks and the notes inside it stay trashed and must be restored individually.",
  usage: 'joplin-axi notebooks restore <id>',
  flags: {},
  examples: ['joplin-axi notebooks restore c8a068acf54642a9b50f7f5a45195e2a'],
};

async function runRestore(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notebooks restore <id>');

  await client.restoreNotebook(id);
  return sections(
    object('notebook', { id, restored: true }),
    help([
      'Run `joplin-axi notebooks list` to confirm.',
      'Sub-notebooks and notes inside stay trashed — restore each individually (`notes list --trash` to find them).',
    ])
  );
}

export const notebooksCommands: Record<string, Command> = {
  list: { spec: listSpec, run: runList },
  create: { spec: createSpec, run: runCreate },
  update: { spec: updateSpec, run: runUpdate },
  delete: { spec: deleteSpec, run: runDelete },
  restore: { spec: restoreSpec, run: runRestore },
};
