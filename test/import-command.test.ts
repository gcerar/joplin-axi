import { promises as fs } from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { importCommand } from '../src/commands/import.js';
import { UsageError } from '../src/lib/args.js';
import { args, fakeClientFactory, textOf } from './helpers.js';

let dir: string;

beforeEach(async () => {
  dir = await fs.mkdtemp(path.join(os.tmpdir(), 'joplin-axi-import-cmd-test-'));
});

afterEach(async () => {
  await fs.rm(dir, { recursive: true, force: true });
});

// No client methods stubbed — any accidental Joplin API call throws
// immediately (undefined is not a function), which is exactly the assertion
// a --dry-run test wants: zero writes, in fact zero client calls at all.
const noOpClient = fakeClientFactory({});

describe('import — validation', () => {
  it('rejects a nonexistent path', async () => {
    const client = noOpClient();
    await expect(importCommand.run(args([path.join(dir, 'missing.md')], { notebook: 'nb1' }), client)).rejects.toThrow(UsageError);
  });

  it('requires --notebook for a real (non-dry-run) md import', async () => {
    const file = path.join(dir, 'note.md');
    await fs.writeFile(file, '# Title\n\nBody');
    const client = noOpClient();
    await expect(importCommand.run(args([file]), client)).rejects.toThrow(/--notebook is required/);
  });

  it('rejects an invalid --format value', async () => {
    const file = path.join(dir, 'note.md');
    await fs.writeFile(file, 'Body');
    const client = noOpClient();
    await expect(importCommand.run(args([file], { notebook: 'nb1', format: 'csv' }), client)).rejects.toThrow(UsageError);
  });

  it('rejects an invalid --on-duplicate value', async () => {
    const file = path.join(dir, 'note.md');
    await fs.writeFile(file, 'Body');
    const client = noOpClient();
    await expect(importCommand.run(args([file], { notebook: 'nb1', 'on-duplicate': 'overwrite' }), client)).rejects.toThrow(UsageError);
  });
});

describe('import — format auto-detection', () => {
  it('detects jex from the .jex extension', async () => {
    const file = path.join(dir, 'export.jex');
    await fs.writeFile(file, 'not a real tar, but we only need format detection + dry-run to short-circuit before parsing');
    const client = noOpClient();
    // Invalid tar content still fails during parseJexSource — this test only
    // confirms .jex routes to the jex parser (it errors from *there*, not
    // from a "not a markdown file" error, which would indicate md was picked).
    await expect(importCommand.run(args([file], { 'dry-run': true }), client)).rejects.toThrow(/JEX archive/);
  });

  it('detects md for anything else, including a directory', async () => {
    await fs.writeFile(path.join(dir, 'note.md'), '# Title\n\nBody');
    const client = noOpClient();
    const out = textOf(await importCommand.run(args([dir], { 'dry-run': true }), client));
    expect(out).toContain('format: md');
  });
});

describe('import — dry-run', () => {
  it('reports counts and makes zero Joplin API calls', async () => {
    await fs.writeFile(path.join(dir, 'a.md'), '# A\n\nBody A');
    await fs.writeFile(path.join(dir, 'b.md'), '---\ntags: [work]\n---\n# B\n\nBody B');
    const client = noOpClient();

    const out = textOf(await importCommand.run(args([dir], { 'dry-run': true }), client));

    expect(out).toContain('format: md');
    expect(out).toContain('notes: 2');
    expect(out).toContain('dry run');
  });

  it('does not require --notebook when --dry-run is set', async () => {
    await fs.writeFile(path.join(dir, 'a.md'), '# A\n\nBody A');
    const client = noOpClient();
    // Should not throw despite no --notebook, since --dry-run never writes.
    await importCommand.run(args([dir], { 'dry-run': true }), client);
  });
});
