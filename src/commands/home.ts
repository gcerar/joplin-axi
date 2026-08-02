import type { JoplinClient } from '../client.js';
import { fmtTime, help, object, sections, table } from '../toon.js';

// The no-args view (AXI principle 8: content-first). Shows live state instead
// of a usage manual — connectivity, notebook/tag counts, and recent notes.
export async function homeView(client: JoplinClient): Promise<string> {
  const clipperOk = await client.ping();
  if (!clipperOk) {
    return sections(
      object('joplin-axi', { bin: 'joplin-axi', clipper: 'unreachable' }),
      help(['Check that Joplin is running and Web Clipper is enabled (Tools → Options → Web Clipper).'])
    );
  }

  const [notebooks, tags, recent] = await Promise.all([
    client.listNotebooks(),
    client.listTags(),
    client.listNotes({ fields: ['id', 'title', 'parent_id', 'updated_time'], limit: 5 }),
  ]);

  const summary = object('joplin-axi', {
    bin: 'joplin-axi',
    clipper: 'reachable',
    notebooks: notebooks.length,
    tags: tags.length,
  });

  const rows = recent.items.map((n) => ({
    id: n.id,
    title: n.title,
    updated: fmtTime(n.updated_time),
  }));

  const recentTable = rows.length ? table('recent_notes', ['id', 'title', 'updated'], rows) : 'recent_notes: 0 notes found';

  const hints = help([
    'Run `joplin-axi notes list` to browse notes.',
    'Run `joplin-axi notebooks list` to see the notebook tree.',
    'Run `joplin-axi --help` for the full command reference.',
  ]);

  return sections(summary, recentTable, hints);
}
