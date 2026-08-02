import type { JoplinClient } from '../client.js';
import type { Command } from '../lib/command.js';
import { type CommandSpec, type ParsedArgs, requirePositional, splitList, UsageError } from '../lib/args.js';
import { CHECK_SCOPES_HINT, resolveNoteScope } from '../lib/note-scope.js';
import { resolveTagId } from '../lib/tag-lookup.js';
import { fmtTime, help, object, sections, table, truncate } from '../toon.js';

const NOTE_LINK_RE = /\[([^\]]*)\]\(([^)]+)\)/g;
const INTERNAL_TARGET_RE = /^:\/[0-9a-f]{32}/i;

// Plain literal replacement of the first occurrence only — deliberately not
// `haystack.replace(needle, replacement)`, since JS treats $&/$$/$`/$' in a
// *string* replacement argument as special patterns even when the search
// value is a plain string, not a regex. That previously made --find/--replace
// behave differently (and silently wrong) depending on whether --all was also
// passed, since the --all path already used literal split/join.
function replaceFirst(haystack: string, needle: string, replacement: string): string {
  const idx = haystack.indexOf(needle);
  return haystack.slice(0, idx) + replacement + haystack.slice(idx + needle.length);
}

// ── notes list ───────────────────────────────────────────────────────────────

const DEFAULT_LIST_FIELDS = ['id', 'title', 'notebook', 'updated'];
const AVAILABLE_LIST_FIELDS = ['id', 'title', 'notebook', 'updated', 'created', 'is_todo', 'deleted'];

const listSpec: CommandSpec = {
  name: 'notes list',
  summary: 'List or search notes. --query/--notebook/--tag/--task can all be combined (intersected).',
  usage: 'joplin-axi notes list [--query <text>] [--notebook <id>] [--tag <title>] [--task] [--trash] [--limit <n>] [--fields <list>]',
  flags: {
    query: { type: 'string', description: 'Free-text search (Joplin search syntax); combinable with --notebook/--tag/--task' },
    notebook: { type: 'string', description: 'Restrict to a notebook ID; combinable with --query/--tag/--task' },
    tag: { type: 'string', description: 'Restrict to a tag title; combinable with --query/--notebook/--task' },
    task: { type: 'boolean', description: 'Restrict to to-do notes; combinable with any other filter', default: false },
    trash: {
      type: 'boolean',
      description: 'List only trashed notes (cannot combine with --query/--task/--notebook/--tag)',
      default: false,
    },
    limit: { type: 'number', description: 'Max notes to return', default: 20 },
    fields: { type: 'string', description: `Comma-separated output fields (${AVAILABLE_LIST_FIELDS.join(',')})` },
  },
  examples: [
    'joplin-axi notes list --notebook a1b2c3',
    'joplin-axi notes list --query "annual report"',
    'joplin-axi notes list --task --limit 50',
    'joplin-axi notes list --notebook a1b2c3 --query "meeting" --task',
    'joplin-axi notes list --tag active --query "report"',
    'joplin-axi notes list --trash',
  ],
};

async function runList(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const query = parsed.flags.query as string | undefined;
  const notebook = parsed.flags.notebook as string | undefined;
  const tag = parsed.flags.tag as string | undefined;
  const task = Boolean(parsed.flags.task);
  const trash = Boolean(parsed.flags.trash);
  const limit = Number(parsed.flags.limit);
  if (limit <= 0) {
    throw new UsageError('--limit must be a positive number', [listSpec.usage]);
  }

  const usingDefaultFields = !parsed.flags.fields;
  const fields = usingDefaultFields
    ? trash
      ? [...DEFAULT_LIST_FIELDS, 'deleted']
      : DEFAULT_LIST_FIELDS
    : splitList(parsed.flags.fields);
  for (const f of fields) {
    if (!AVAILABLE_LIST_FIELDS.includes(f)) {
      throw new UsageError(`unknown field \`${f}\` for \`notes list\``, [`valid fields: ${AVAILABLE_LIST_FIELDS.join(', ')}`]);
    }
  }

  if (trash && (query || task || notebook || tag)) {
    throw new UsageError('--trash cannot be combined with --query/--task/--notebook/--tag', [
      'Joplin only documents include_deleted for the unfiltered /notes listing (not /search, /folders, or /tags)',
    ]);
  }

  const tagId = tag ? await resolveTagId(client, tag) : undefined;

  const apiFields = new Set<string>(['id', 'title', 'parent_id', 'updated_time']);
  if (fields.includes('created')) apiFields.add('created_time');
  if (fields.includes('is_todo') || task) apiFields.add('is_todo');
  if (trash || fields.includes('deleted')) apiFields.add('deleted_time');
  const apiFieldsArr = Array.from(apiFields);

  let items: Record<string, any>[];

  if (trash) {
    // include_deleted mixes trashed notes into the normal result set rather than
    // listing only them, so fetch a larger raw batch and filter/slice client-side
    // to approximate "list only trashed notes" (matching joplin-mcp's trash=True UX).
    const { items: rawItems } = await client.listNotes({
      fields: apiFieldsArr,
      limit: Math.max(limit * 20, 500),
      includeDeleted: true,
    });
    items = rawItems.filter((n) => Number(n.deleted_time) > 0).slice(0, limit);
  } else if (notebook || tagId || query || task) {
    // See src/lib/note-scope.ts for why intersecting ID-scoped sets is safe
    // where interpolating a notebook/tag title into the search DSL isn't
    // (confirmed unsafe — a live test produced a silent false-negative; TODO.md).
    const scoped = await resolveNoteScope(client, {
      notebookId: notebook,
      tagId,
      query,
      task,
      fields: apiFieldsArr,
      searchCap: Math.max(limit * 20, 500),
    });
    items = scoped.slice(0, limit);
  } else {
    items = (await client.listNotes({ fields: apiFieldsArr, limit })).items;
  }

  let notebookNames: Record<string, string> = {};
  if (fields.includes('notebook') && items.length) {
    const nbs = await client.listNotebooks();
    notebookNames = Object.fromEntries(nbs.map((n) => [n.id, n.title]));
  }

  const rows = items.map((n) => ({
    id: n.id,
    title: n.title,
    notebook: notebookNames[n.parent_id] ?? n.parent_id,
    updated: fmtTime(n.updated_time),
    created: fmtTime(n.created_time),
    is_todo: n.is_todo ? 'yes' : 'no',
    deleted: fmtTime(n.deleted_time),
  }));

  const scopeDescription = [trash && 'trashed', task && 'to-do'].filter(Boolean).join(' ');
  const contextParts = [
    query ? `query \`${query}\`` : '',
    notebook ? `notebook \`${notebook}\`` : '',
    tag ? `tag \`${tag}\`` : '',
  ].filter(Boolean);
  const context = contextParts.length ? ` for ${contextParts.join(', ')}` : '';
  const body = rows.length
    ? table('notes', fields, rows)
    : `notes: 0 ${scopeDescription ? `${scopeDescription} ` : ''}notes found${context}`;

  const hints = rows.length
    ? [
        'Run `joplin-axi notes get <id>` for the full note.',
        rows.length >= limit ? `Run with --limit ${limit * 2} to see more.` : '',
      ].filter(Boolean)
    : [CHECK_SCOPES_HINT];

  return sections(body, help(hints));
}

// ── notes get ────────────────────────────────────────────────────────────────

const DEFAULT_GET_FIELDS = ['id', 'title', 'notebook', 'updated', 'created', 'is_todo', 'body'];

const getSpec: CommandSpec = {
  name: 'notes get',
  summary: 'Fetch a single note by ID.',
  usage: 'joplin-axi notes get <id> [--full] [--fields <list>]',
  flags: {
    full: { type: 'boolean', description: 'Show the full body instead of a truncated preview', default: false },
    fields: { type: 'string', description: `Comma-separated output fields (${DEFAULT_GET_FIELDS.join(',')})` },
  },
  examples: ['joplin-axi notes get 3f9c2a1b', 'joplin-axi notes get 3f9c2a1b --full'],
};

async function runGet(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notes get <id>');

  const fields = parsed.flags.fields ? splitList(parsed.flags.fields) : DEFAULT_GET_FIELDS;
  for (const f of fields) {
    if (!DEFAULT_GET_FIELDS.includes(f)) {
      throw new UsageError(`unknown field \`${f}\` for \`notes get\``, [`valid fields: ${DEFAULT_GET_FIELDS.join(', ')}`]);
    }
  }

  // Only fetch the API fields actually needed for the requested display fields
  // — mirrors notes list's apiFields pattern instead of always pulling the full
  // (potentially large) body regardless of --fields.
  const apiFields = new Set<string>();
  if (fields.includes('id')) apiFields.add('id');
  if (fields.includes('title')) apiFields.add('title');
  if (fields.includes('notebook')) apiFields.add('parent_id');
  if (fields.includes('updated')) apiFields.add('updated_time');
  if (fields.includes('created')) apiFields.add('created_time');
  if (fields.includes('is_todo')) apiFields.add('is_todo');
  if (fields.includes('body')) apiFields.add('body');

  const note = await client.getNote(id, Array.from(apiFields));
  const notebooks = fields.includes('notebook') ? await client.listNotebooks() : [];
  const notebookName = notebooks.find((n) => n.id === note.parent_id)?.title ?? note.parent_id;

  const full = Boolean(parsed.flags.full);
  const bodyText = note.body ?? '';
  const { text: shownBody, truncated, total } = full ? { text: bodyText, truncated: false, total: bodyText.length } : truncate(bodyText, 800);

  const out: Record<string, unknown> = {};
  if (fields.includes('id')) out.id = note.id;
  if (fields.includes('title')) out.title = note.title;
  if (fields.includes('notebook')) out.notebook = notebookName;
  if (fields.includes('updated')) out.updated = fmtTime(note.updated_time);
  if (fields.includes('created')) out.created = fmtTime(note.created_time);
  if (fields.includes('is_todo')) out.is_todo = note.is_todo ? 'yes' : 'no';
  if (fields.includes('body')) out.body = shownBody;

  const parts = [object('note', out)];
  if (truncated) {
    parts.push(help([`Run \`joplin-axi notes get ${id} --full\` to see the complete body (${total} chars total).`]));
  }
  return sections(...parts);
}

// ── notes find-in ────────────────────────────────────────────────────────────

const findInSpec: CommandSpec = {
  name: 'notes find-in',
  summary: "Regex search within a single note's body (line-based, with context).",
  usage: 'joplin-axi notes find-in <id> <pattern> [--ignore-case] [--limit <n>]',
  flags: {
    'ignore-case': { type: 'boolean', description: 'Case-insensitive match', default: false },
    limit: { type: 'number', description: 'Max matches to return', default: 20 },
  },
  examples: ['joplin-axi notes find-in 3f9c2a1b "TODO:.*"'],
};

async function runFindIn(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const [id, pattern] = parsed.positionals;
  if (!id || !pattern) {
    throw new UsageError('requires <id> and <pattern>', ['joplin-axi notes find-in <id> <pattern>']);
  }

  const note = await client.getNote(id, ['id', 'body']);
  const lines = String(note.body ?? '').split('\n');

  const flags = 'g' + (parsed.flags['ignore-case'] ? 'i' : '');
  let re: RegExp;
  try {
    re = new RegExp(pattern, flags);
  } catch (e) {
    throw new UsageError(`invalid regex: ${(e as Error).message}`);
  }

  const limit = Number(parsed.flags.limit);
  if (limit <= 0) {
    throw new UsageError('--limit must be a positive number', [findInSpec.usage]);
  }

  const CONTEXT_LIMIT = 120;
  let anyContextTruncated = false;
  const rows: Record<string, unknown>[] = [];
  for (let i = 0; i < lines.length && rows.length < limit; i++) {
    re.lastIndex = 0;
    const m = re.exec(lines[i]);
    if (m) {
      const { text: context, truncated } = truncate(lines[i].trim(), CONTEXT_LIMIT);
      if (truncated) anyContextTruncated = true;
      rows.push({ line: i + 1, match: m[0], context });
    }
  }

  const body = rows.length ? table('matches', ['line', 'match', 'context'], rows) : `matches: 0 matches for /${pattern}/ in note ${id}`;
  const hints = rows.length
    ? [
        anyContextTruncated
          ? `Some context lines were truncated; run \`joplin-axi notes get ${id} --full\` to see complete lines.`
          : `Run \`joplin-axi notes get ${id} --full\` to see these matches in full context.`,
      ]
    : [];

  return sections(body, help(hints));
}

// ── notes links ──────────────────────────────────────────────────────────────

const linksSpec: CommandSpec = {
  name: 'notes links',
  summary: "Extract markdown links from a note's body.",
  usage: 'joplin-axi notes links <id>',
  flags: {},
  examples: ['joplin-axi notes links 3f9c2a1b'],
};

async function runLinks(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notes links <id>');

  const note = await client.getNote(id, ['id', 'body']);
  const rows: Record<string, unknown>[] = [];

  let m: RegExpExecArray | null;
  NOTE_LINK_RE.lastIndex = 0;
  while ((m = NOTE_LINK_RE.exec(String(note.body ?? '')))) {
    const [, text, target] = m;
    rows.push({ text, target, type: INTERNAL_TARGET_RE.test(target) ? 'note' : 'external' });
  }

  const body = rows.length ? table('links', ['text', 'target', 'type'], rows) : `links: 0 links found in note ${id}`;
  const hasInternalLink = rows.some((r) => r.type === 'note');
  const hints = hasInternalLink
    ? ["Run `joplin-axi notes get <id>` (strip the leading `:/` from an internal link's target) to view a linked note."]
    : [];

  return sections(body, help(hints));
}

// ── notes resources ──────────────────────────────────────────────────────────

const DEFAULT_RESOURCE_FIELDS = ['id', 'title', 'mime', 'size'];
const AVAILABLE_RESOURCE_FIELDS = ['id', 'title', 'mime', 'size', 'ocr_text'];
const OCR_PREVIEW_LIMIT = 500;

const resourcesSpec: CommandSpec = {
  name: 'notes resources',
  summary: "List a note's attached resources (images, PDFs, attachments).",
  usage: 'joplin-axi notes resources <id> [--fields <list>] [--full]',
  flags: {
    fields: { type: 'string', description: `Comma-separated output fields (${AVAILABLE_RESOURCE_FIELDS.join(',')})` },
    full: { type: 'boolean', description: 'Show complete OCR text instead of a truncated preview', default: false },
  },
  examples: ['joplin-axi notes resources 3f9c2a1b', 'joplin-axi notes resources 3f9c2a1b --fields id,title,ocr_text --full'],
};

async function runResources(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notes resources <id>');

  const fields = parsed.flags.fields ? splitList(parsed.flags.fields) : DEFAULT_RESOURCE_FIELDS;
  for (const f of fields) {
    if (!AVAILABLE_RESOURCE_FIELDS.includes(f)) {
      throw new UsageError(`unknown field \`${f}\` for \`notes resources\``, [`valid fields: ${AVAILABLE_RESOURCE_FIELDS.join(', ')}`]);
    }
  }

  const full = Boolean(parsed.flags.full);
  // Only request ocr_text from the API when it's actually going to be shown —
  // it can be large, and previously was always fetched regardless of --fields.
  const resources = await client.getNoteResources(id, fields);

  let anyTruncated = false;
  const rows = resources.map((r) => {
    let ocrText = '';
    if (fields.includes('ocr_text') && r.ocr_text) {
      const preview = full ? { text: String(r.ocr_text), truncated: false } : truncate(String(r.ocr_text), OCR_PREVIEW_LIMIT);
      ocrText = preview.text;
      if (preview.truncated) anyTruncated = true;
    }
    return { id: r.id, title: r.title, mime: r.mime, size: r.size, ocr_text: ocrText };
  });

  const body = rows.length ? table('resources', fields, rows) : `resources: 0 resources attached to note ${id}`;
  const hints = anyTruncated ? [`Run \`joplin-axi notes resources ${id} --full\` to see complete OCR text.`] : [];

  return sections(body, help(hints));
}

// ── notes create ─────────────────────────────────────────────────────────────

const createSpec: CommandSpec = {
  name: 'notes create',
  summary: 'Create a new note.',
  usage: 'joplin-axi notes create --title <text> [--body <text>] [--notebook <id>]',
  flags: {
    title: { type: 'string', description: 'Note title (required)' },
    body: { type: 'string', description: 'Note body (Markdown)' },
    notebook: { type: 'string', description: "Notebook ID (omit for Joplin's default notebook)" },
  },
  examples: [
    'joplin-axi notes create --title "Meeting notes" --body "# Agenda"',
    'joplin-axi notes create --title "Quick capture" --notebook b3b3d60013f04ccf8ad373cf7b2fc4d1',
  ],
};

async function runCreate(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const title = parsed.flags.title as string | undefined;
  if (!title) {
    throw new UsageError('--title is required', ['joplin-axi notes create --title <text> [--body <text>] [--notebook <id>]']);
  }

  const fields: Record<string, unknown> = { title };
  if (parsed.flags.body !== undefined) fields.body = parsed.flags.body;
  if (parsed.flags.notebook !== undefined) fields.parent_id = parsed.flags.notebook;

  const note = await client.createNote(fields);
  return sections(
    object('note', { id: note.id, title: note.title }),
    help([`Run \`joplin-axi notes get ${note.id}\` to see the full note.`])
  );
}

// ── notes update ─────────────────────────────────────────────────────────────

const updateSpec: CommandSpec = {
  name: 'notes update',
  summary: "Update a note's title, body, and/or notebook (moves the note if --notebook is given).",
  usage: 'joplin-axi notes update <id> [--title <text>] [--body <text>] [--notebook <id>]',
  flags: {
    title: { type: 'string', description: 'New title' },
    body: { type: 'string', description: 'New body (Markdown) — replaces the entire body' },
    notebook: { type: 'string', description: 'Move the note to this notebook ID' },
  },
  examples: ['joplin-axi notes update 3f9c2a1b --title "Renamed"', 'joplin-axi notes update 3f9c2a1b --notebook 810dc26ff91b4133b7bc13532a9c3bdd'],
};

async function runUpdate(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notes update <id> [--title] [--body] [--notebook]');

  const fields: Record<string, unknown> = {};
  if (parsed.flags.title !== undefined) fields.title = parsed.flags.title;
  if (parsed.flags.body !== undefined) fields.body = parsed.flags.body;
  if (parsed.flags.notebook !== undefined) fields.parent_id = parsed.flags.notebook;

  if (Object.keys(fields).length === 0) {
    throw new UsageError('nothing to update — pass at least one of --title/--body/--notebook', [
      'joplin-axi notes update <id> [--title] [--body] [--notebook]',
    ]);
  }

  const note = await client.updateNote(id, fields);
  return sections(
    object('note', { id: note.id ?? id, title: note.title, updated: 'ok' }),
    help([`Run \`joplin-axi notes get ${id}\` to see the updated note.`])
  );
}

// ── notes edit ───────────────────────────────────────────────────────────────

const editSpec: CommandSpec = {
  name: 'notes edit',
  summary: "Precision-edit a note's body: find/replace, append, or prepend.",
  usage: 'joplin-axi notes edit <id> [--find <text> --replace <text>] [--append <text>] [--prepend <text>] [--all]',
  flags: {
    find: { type: 'string', description: 'Exact text to find (used with --replace)' },
    replace: { type: 'string', description: 'Replacement text (used with --find)' },
    append: { type: 'string', description: 'Text to add to the end of the body' },
    prepend: { type: 'string', description: 'Text to add to the start of the body' },
    all: { type: 'boolean', description: 'With --find/--replace, replace all occurrences instead of just the first', default: false },
  },
  examples: ['joplin-axi notes edit 3f9c2a1b --find "TODO" --replace "DONE"', 'joplin-axi notes edit 3f9c2a1b --append "\\n\\n## Update\\nDone."'],
};

async function runEdit(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notes edit <id> ...');

  const find = parsed.flags.find as string | undefined;
  const replace = parsed.flags.replace as string | undefined;
  const append = parsed.flags.append as string | undefined;
  const prepend = parsed.flags.prepend as string | undefined;

  const modesUsed = [find !== undefined, append !== undefined, prepend !== undefined].filter(Boolean).length;
  if (modesUsed === 0) {
    throw new UsageError('requires one of --find/--replace, --append, or --prepend', [
      'joplin-axi notes edit <id> [--find <text> --replace <text>] [--append <text>] [--prepend <text>]',
    ]);
  }
  if (modesUsed > 1) {
    throw new UsageError('--find/--replace, --append, and --prepend are mutually exclusive', ['pass exactly one edit mode per call']);
  }
  if (find !== undefined && replace === undefined) {
    throw new UsageError('--find requires --replace', ['joplin-axi notes edit <id> --find <text> --replace <text>']);
  }
  if (find === '') {
    throw new UsageError('--find must not be empty', ['an empty search string matches everywhere and would corrupt the body']);
  }
  if (replace !== undefined && find === undefined) {
    throw new UsageError('--replace requires --find', ['joplin-axi notes edit <id> --find <text> --replace <text>']);
  }
  if (parsed.flags.all && find === undefined) {
    throw new UsageError('--all only applies with --find/--replace', [
      'joplin-axi notes edit <id> --find <text> --replace <text> --all',
    ]);
  }

  const note = await client.getNote(id, ['id', 'body']);
  const body = String(note.body ?? '');

  let newBody: string;
  if (find !== undefined) {
    if (!body.includes(find)) {
      throw new UsageError(`text not found in note ${id}: ${JSON.stringify(find)}`, ['run `joplin-axi notes get ' + id + '` to check the current body']);
    }
    newBody = parsed.flags.all ? body.split(find).join(replace as string) : replaceFirst(body, find, replace as string);
  } else if (append !== undefined) {
    newBody = body + append;
  } else {
    newBody = (prepend as string) + body;
  }

  await client.updateNote(id, { body: newBody });
  return sections(
    object('note', { id, edited: true, length: newBody.length }),
    help([`Run \`joplin-axi notes get ${id} --full\` to see the edited body.`])
  );
}

// ── notes delete ─────────────────────────────────────────────────────────────

const deleteSpec: CommandSpec = {
  name: 'notes delete',
  summary: "Move a note to Joplin's trash. Always a soft delete — joplin-axi never permanently deletes.",
  usage: 'joplin-axi notes delete <id>',
  flags: {},
  examples: ['joplin-axi notes delete 3f9c2a1b'],
};

async function runDelete(parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const id = requirePositional(parsed, 0, 'id', 'joplin-axi notes delete <id>');

  await client.deleteNote(id);
  return sections(
    object('note', { id, trashed: true }),
    help(['Recoverable from Joplin\'s trash in the app itself — joplin-axi has no restore command yet (see TODO.md).'])
  );
}

export const notesCommands: Record<string, Command> = {
  list: { spec: listSpec, run: runList },
  get: { spec: getSpec, run: runGet },
  'find-in': { spec: findInSpec, run: runFindIn },
  links: { spec: linksSpec, run: runLinks },
  resources: { spec: resourcesSpec, run: runResources },
  create: { spec: createSpec, run: runCreate },
  update: { spec: updateSpec, run: runUpdate },
  edit: { spec: editSpec, run: runEdit },
  delete: { spec: deleteSpec, run: runDelete },
};
