import type { JoplinClient } from '../client.js';
import { UsageError } from './args.js';

/** Resolves a tag *title* to its ID — shared by `notes list --tag` and `tags` commands. */
export async function resolveTagId(client: JoplinClient, title: string): Promise<string> {
  const allTags = await client.listTags();
  const match = allTags.find((t) => t.title === title);
  if (!match) {
    throw new UsageError(`no tag titled \`${title}\``, [
      'run `joplin-axi tags list` to see available tags, or `joplin-axi tags create <title>` to make one',
    ]);
  }
  return match.id;
}
