#!/usr/bin/env node
import { JoplinClient } from './client.js';
import { homeView } from './commands/home.js';
import { importCommand } from './commands/import.js';
import { notebooksCommands } from './commands/notebooks.js';
import { notesCommands } from './commands/notes.js';
import { pingCommand } from './commands/ping.js';
import { tagsCommands } from './commands/tags.js';
import { helpText, parseArgs, UsageError } from './lib/args.js';
import type { Command, CommandOutput } from './lib/command.js';
import { errorOut } from './toon.js';

function printResult(result: string | CommandOutput): void {
  if (typeof result === 'string') {
    console.log(result);
    return;
  }
  console.log(result.output);
  if (result.exitCode !== 0) process.exitCode = result.exitCode;
}

const GROUPS: Record<string, Record<string, Command>> = {
  notes: notesCommands,
  notebooks: notebooksCommands,
  tags: tagsCommands,
};

// Single-verb commands that don't fit the <group> <command> shape.
const TOP_LEVEL_COMMANDS: Record<string, Command> = {
  ping: pingCommand,
  import: importCommand,
};

const TOP_LEVEL_HELP = `joplin-axi — AXI-style CLI for Joplin

usage: joplin-axi <group> <command> [flags]
       joplin-axi ping
       joplin-axi import <path> [flags]
       joplin-axi

groups:
  notes       list, get, find-in, links, resources, create, update, edit, delete, restore
  notebooks   list, create, update, delete, restore
  tags        list, of, create, update, delete, add, remove

Run \`joplin-axi <group> <command> --help\` or \`joplin-axi import --help\` for details.`;

function requireEnv(): { baseUrl: string; token: string } {
  const token = process.env.JOPLIN_TOKEN;
  if (!token) {
    console.log(
      errorOut('JOPLIN_TOKEN environment variable is required', [
        'export JOPLIN_TOKEN=<token from Joplin → Options → Web Clipper>',
      ])
    );
    process.exit(1);
  }
  const baseUrl = process.env.JOPLIN_BASE_URL || 'http://localhost:41184';
  return { baseUrl, token };
}

async function main(): Promise<void> {
  const argv = process.argv.slice(2);

  if (argv.length === 0) {
    const client = new JoplinClient(requireEnv());
    console.log(await homeView(client));
    return;
  }

  const [first, ...rest] = argv;

  if (first === '--help' || first === '-h') {
    console.log(TOP_LEVEL_HELP);
    return;
  }

  const topLevel = TOP_LEVEL_COMMANDS[first];
  if (topLevel) {
    const parsed = parseArgs(rest, topLevel.spec);
    if (parsed.help) {
      console.log(helpText(topLevel.spec));
      return;
    }
    const client = new JoplinClient(requireEnv());
    printResult(await topLevel.run(parsed, client));
    return;
  }

  const group = GROUPS[first];
  if (!group) {
    console.log(
      errorOut(`unknown command \`${first}\``, [`valid commands: ${[...Object.keys(GROUPS), ...Object.keys(TOP_LEVEL_COMMANDS)].join(', ')}`])
    );
    process.exit(2);
  }

  const [sub, ...cmdArgv] = rest;
  const command = sub ? group[sub] : undefined;
  if (!command) {
    const attempted = sub ? `${first} ${sub}` : first;
    console.log(errorOut(`unknown command \`${attempted}\``, [`valid subcommands for \`${first}\`: ${Object.keys(group).join(', ')}`]));
    process.exit(2);
  }

  const parsed = parseArgs(cmdArgv, command.spec);
  if (parsed.help) {
    console.log(helpText(command.spec));
    return;
  }

  const client = new JoplinClient(requireEnv());
  printResult(await command.run(parsed, client));
}

main().catch((err: unknown) => {
  if (err instanceof UsageError) {
    console.log(errorOut(err.message, err.helpLines));
    process.exit(2);
  }
  console.log(errorOut(err instanceof Error ? err.message : String(err)));
  process.exit(1);
});
