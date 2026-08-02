import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import type { JoplinClient } from '../client.js';
import type { Command, CommandOutput } from '../lib/command.js';
import { type CommandSpec, type ParsedArgs, requirePositional, UsageError } from '../lib/args.js';
import { runImport } from '../lib/import/importer.js';
import { parseJexSource } from '../lib/import/jex-source.js';
import { parseMarkdownSource } from '../lib/import/markdown-source.js';
import type { OnDuplicate, ParsedImport } from '../lib/import/types.js';
import { help, object, sections } from '../toon.js';

const spec: CommandSpec = {
  name: 'import',
  summary: 'Import notes from a markdown file/directory or a Joplin .jex export.',
  usage: 'joplin-axi import <path> [--notebook <id>] [--format md|jex] [--on-duplicate skip|rename] [--dry-run]',
  flags: {
    notebook: {
      type: 'string',
      description: 'Target notebook ID — required for md (no default target), an optional graft point for jex',
    },
    format: { type: 'string', description: 'Source format: md or jex (auto-detected from the path extension if omitted)' },
    'on-duplicate': {
      type: 'string',
      description: 'skip (default) or rename a note whose title already exists in the target notebook',
      default: 'skip',
    },
    'dry-run': { type: 'boolean', description: 'Parse and report counts only — no Joplin writes', default: false },
  },
  examples: [
    'joplin-axi import ./notes --notebook a1b2c3',
    'joplin-axi import export.jex --notebook a1b2c3',
    'joplin-axi import export.jex --dry-run',
    'joplin-axi import ./notes --notebook a1b2c3 --on-duplicate rename',
  ],
};

function detectFormat(sourcePath: string): 'md' | 'jex' {
  return path.extname(sourcePath).toLowerCase() === '.jex' ? 'jex' : 'md';
}

async function run(parsed: ParsedArgs, client: JoplinClient): Promise<string | CommandOutput> {
  const sourcePath = requirePositional(parsed, 0, 'path', spec.usage);
  const notebook = parsed.flags.notebook as string | undefined;
  const dryRun = Boolean(parsed.flags['dry-run']);

  const onDuplicate = (parsed.flags['on-duplicate'] as string | undefined) ?? 'skip';
  if (onDuplicate !== 'skip' && onDuplicate !== 'rename') {
    throw new UsageError(`--on-duplicate must be \`skip\` or \`rename\`, got \`${onDuplicate}\``, [spec.usage]);
  }

  const format = (parsed.flags.format as string | undefined) ?? detectFormat(sourcePath);
  if (format !== 'md' && format !== 'jex') {
    throw new UsageError(`--format must be \`md\` or \`jex\`, got \`${format}\``, [spec.usage]);
  }

  try {
    await fs.access(sourcePath);
  } catch {
    throw new UsageError(`path does not exist: ${sourcePath}`, [spec.usage]);
  }

  // No default "Imported" target like the reference — importing without
  // confirming a destination is exactly the kind of surprise this project's
  // safety conventions want to avoid. jex carries its own notebook
  // hierarchy, so --notebook there is just an optional graft point. Only
  // enforced for a real run — a --dry-run is read-only and useful precisely
  // for deciding on a target notebook before committing to one.
  if (format === 'md' && !notebook && !dryRun) {
    throw new UsageError('--notebook is required for md imports', [
      'run `joplin-axi notebooks list` to find a target, or `joplin-axi notebooks create <title>` to make one',
    ]);
  }

  const parsedImport: ParsedImport = format === 'jex' ? await parseJexSource(sourcePath) : await parseMarkdownSource(sourcePath);

  const counts = {
    format,
    notebooks: parsedImport.notebooks.length,
    tags: parsedImport.tags.length,
    notes: parsedImport.notes.length,
    resources: parsedImport.resources.length,
  };

  if (dryRun) {
    return sections(
      object('import', counts),
      help(['This was a dry run — nothing was written to Joplin. Re-run without --dry-run to apply.'])
    );
  }

  const report = await runImport(parsedImport, client, { targetNotebookId: notebook, onDuplicate: onDuplicate as OnDuplicate });

  const output = sections(
    object('import', {
      format,
      notebooks_created: report.notebooksCreated,
      tags_created: report.tagsCreated,
      notes_created: report.notesCreated,
      notes_skipped: report.notesSkipped.length,
      notes_failed: report.notesFailed.length,
      link_rewrite_failed: report.linkRewriteFailed.length,
      resources_uploaded: report.resourcesUploaded,
      unresolved_links: report.unresolvedLinks,
    }),
    help(
      [
        report.notebooksCreated ? 'Run `joplin-axi notebooks list` to see the new notebook structure.' : '',
        report.notesFailed.length
          ? `${report.notesFailed.length} note(s) failed — see notes_failed above; nothing else in the batch was rolled back.`
          : '',
        report.linkRewriteFailed.length
          ? `${report.linkRewriteFailed.length} note(s) were created but their links couldn't be rewritten (see link_rewrite_failed above) — the note bodies may still contain stale :/id references.`
          : '',
        report.unresolvedLinks ? `${report.unresolvedLinks} link(s) in imported notes couldn't be resolved (see the design notes for why).` : '',
      ].filter(Boolean)
    )
  );

  return report.notesFailed.length || report.linkRewriteFailed.length ? { output, exitCode: 1 } : output;
}

export const importCommand: Command = { spec, run };
