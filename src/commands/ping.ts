import type { JoplinClient } from '../client.js';
import type { Command } from '../lib/command.js';
import type { CommandSpec, ParsedArgs } from '../lib/args.js';
import { object } from '../toon.js';

const spec: CommandSpec = {
  name: 'ping',
  summary: 'Check connectivity and authentication against the Joplin Web Clipper API.',
  usage: 'joplin-axi ping',
  flags: {},
  examples: ['joplin-axi ping'],
};

async function run(_parsed: ParsedArgs, client: JoplinClient): Promise<string> {
  const clipper = await client.ping();
  let auth = 'failed';

  if (clipper) {
    try {
      await client.listNotebooks();
      auth = 'ok';
    } catch {
      auth = 'failed';
    }
  }

  return object('ping', { clipper: clipper ? 'reachable' : 'unreachable', auth });
}

export const pingCommand: Command = { spec, run };
