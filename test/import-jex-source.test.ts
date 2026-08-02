import { promises as fs } from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { create as tarCreate } from 'tar';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { parseJexSource } from '../src/lib/import/jex-source.js';
import { ROOT_NOTEBOOK_REF } from '../src/lib/import/types.js';

let dir: string;
let jexPath: string;

beforeEach(async () => {
  dir = await fs.mkdtemp(path.join(os.tmpdir(), 'joplin-axi-jex-test-'));
  jexPath = path.join(dir, 'export.jex');
});

afterEach(async () => {
  await fs.rm(dir, { recursive: true, force: true });
});

/** Builds a real tar archive (via the same `tar` package the implementation
 * uses) laid out like a genuine Joplin RAW export: a folder, two notes (one
 * linking to the other and to a resource), a tag, a note-tag relation, and a
 * resource (metadata item + binary under resources/). */
async function buildFixtureJex(): Promise<void> {
  const src = path.join(dir, 'src');
  await fs.mkdir(path.join(src, 'resources'), { recursive: true });

  const folderMd = ['My Notebook', '', 'id: nb0000000000000000000000000000', 'created_time: 2023-01-01T00:00:00.000Z', 'updated_time: 2023-01-01T00:00:00.000Z', 'type_: 2'].join(
    '\n'
  );
  await fs.writeFile(path.join(src, 'nb0000000000000000000000000000.md'), folderMd);

  const tagMd = ['work', '', 'id: tag000000000000000000000000', 'created_time: 2023-01-01T00:00:00.000Z', 'updated_time: 2023-01-01T00:00:00.000Z', 'type_: 5'].join('\n');
  await fs.writeFile(path.join(src, 'tag000000000000000000000000.md'), tagMd);

  const resourceMetaMd = [
    'photo.png',
    '',
    'id: res0000000000000000000000000',
    'mime: image/png',
    'filename: photo.png',
    'created_time: 2023-01-01T00:00:00.000Z',
    'updated_time: 2023-01-01T00:00:00.000Z',
    'type_: 4',
  ].join('\n');
  await fs.writeFile(path.join(src, 'res0000000000000000000000000.md'), resourceMetaMd);
  await fs.writeFile(path.join(src, 'resources', 'res0000000000000000000000000.png'), Buffer.from([0x89, 0x50, 0x4e, 0x47]));

  const note2Md = [
    'Second Note',
    '',
    'Just some body text, no links.',
    '',
    'id: note2000000000000000000000000',
    'parent_id: nb0000000000000000000000000000',
    'created_time: 2023-02-01T00:00:00.000Z',
    'updated_time: 2023-02-02T00:00:00.000Z',
    'is_todo: 0',
    'todo_completed: 0',
    'type_: 1',
  ].join('\n');
  await fs.writeFile(path.join(src, 'note2000000000000000000000000.md'), note2Md);

  const note1Md = [
    'First Note',
    '',
    'Links to [Second Note](:/note2000000000000000000000000) and an image ![photo](:/res0000000000000000000000000).',
    '',
    'id: note1000000000000000000000000',
    'parent_id: nb0000000000000000000000000000',
    'created_time: 2023-01-05T00:00:00.000Z',
    'updated_time: 2023-01-06T00:00:00.000Z',
    'is_todo: 1',
    'todo_completed: 0',
    'type_: 1',
  ].join('\n');
  await fs.writeFile(path.join(src, 'note1000000000000000000000000.md'), note1Md);

  const relationMd = [
    'note1000000000000000000000000-tag000000000000000000000000',
    '',
    'id: rel0000000000000000000000000',
    'note_id: note1000000000000000000000000',
    'tag_id: tag000000000000000000000000',
    'created_time: 2023-01-05T00:00:00.000Z',
    'updated_time: 2023-01-05T00:00:00.000Z',
    'type_: 6',
  ].join('\n');
  await fs.writeFile(path.join(src, 'rel0000000000000000000000000.md'), relationMd);

  await tarCreate({ file: jexPath, cwd: src, gzip: false }, ['.']);
}

describe('parseJexSource', () => {
  it('rejects a file that is not a valid tar archive', async () => {
    await fs.writeFile(jexPath, 'not a tar archive at all');
    // tar's parser doesn't throw on garbage input, it just yields zero
    // entries — which surfaces as the same "empty or invalid" error as a
    // genuinely empty archive (see the dedicated empty-archive test below).
    await expect(parseJexSource(jexPath)).rejects.toThrow(/empty or invalid JEX archive/);
  });

  it('reconstructs notebooks, tags, note-tag relations, notes, and resources from a real tar archive', async () => {
    await buildFixtureJex();
    const result = await parseJexSource(jexPath);

    expect(result.notebooks).toEqual([{ ref: 'nb0000000000000000000000000000', title: 'My Notebook', parentRef: undefined }]);
    expect(result.tags).toEqual([{ ref: 'tag000000000000000000000000', title: 'work' }]);

    expect(result.notes).toHaveLength(2);
    const first = result.notes.find((n) => n.title === 'First Note')!;
    const second = result.notes.find((n) => n.title === 'Second Note')!;

    expect(first.notebookRef).toBe('nb0000000000000000000000000000');
    expect(first.sourceId).toBe('note1000000000000000000000000');
    expect(first.isTodo).toBe(true);
    expect(first.todoCompleted).toBe(false);
    expect(first.tagRefs).toEqual(['tag000000000000000000000000']);
    expect(first.body).toContain(':/note2000000000000000000000000');
    expect(first.body).toContain(':/res0000000000000000000000000');
    expect(first.createdTime).toBe(Date.parse('2023-01-05T00:00:00.000Z'));

    expect(second.tagRefs).toEqual([]);
    expect(second.notebookRef).toBe('nb0000000000000000000000000000');

    expect(result.resources).toHaveLength(1);
    expect(result.resources[0]).toMatchObject({ id: 'res0000000000000000000000000', filename: 'photo.png', mime: 'image/png' });
    expect(result.resources[0].data).toEqual(Buffer.from([0x89, 0x50, 0x4e, 0x47]));
  });

  it('uses ROOT_NOTEBOOK_REF for a note with no parent_id', async () => {
    const src = path.join(dir, 'src');
    await fs.mkdir(src, { recursive: true });
    const noteMd = ['Orphan Note', '', 'Body.', '', 'id: orphan00000000000000000000000', 'created_time: 2023-01-01T00:00:00.000Z', 'updated_time: 2023-01-01T00:00:00.000Z', 'type_: 1'].join(
      '\n'
    );
    await fs.writeFile(path.join(src, 'orphan.md'), noteMd);
    await tarCreate({ file: jexPath, cwd: src, gzip: false }, ['.']);

    const result = await parseJexSource(jexPath);
    expect(result.notes[0].notebookRef).toBe(ROOT_NOTEBOOK_REF);
  });

  it('rejects an archive with no recognizable note/resource entries', async () => {
    const src = path.join(dir, 'src');
    await fs.mkdir(src, { recursive: true });
    await fs.writeFile(path.join(src, 'readme.txt'), 'not a joplin item');
    await tarCreate({ file: jexPath, cwd: src, gzip: false }, ['.']);

    await expect(parseJexSource(jexPath)).rejects.toThrow(/empty or invalid JEX archive/);
  });

  it('drops a resource metadata item with no matching binary rather than failing', async () => {
    const src = path.join(dir, 'src');
    await fs.mkdir(src, { recursive: true });
    const resourceMetaMd = ['ghost.png', '', 'id: ghost000000000000000000000', 'mime: image/png', 'created_time: 2023-01-01T00:00:00.000Z', 'updated_time: 2023-01-01T00:00:00.000Z', 'type_: 4'].join(
      '\n'
    );
    await fs.writeFile(path.join(src, 'ghost.md'), resourceMetaMd);
    await tarCreate({ file: jexPath, cwd: src, gzip: false }, ['.']);

    const result = await parseJexSource(jexPath);
    expect(result.resources).toEqual([]);
  });
});
