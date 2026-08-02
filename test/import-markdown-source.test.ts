import { promises as fs } from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { parseMarkdownSource, ROOT_NOTEBOOK_REF } from '../src/lib/import/markdown-source.js';

let dir: string;

beforeEach(async () => {
  dir = await fs.mkdtemp(path.join(os.tmpdir(), 'joplin-axi-import-test-'));
});

afterEach(async () => {
  await fs.rm(dir, { recursive: true, force: true });
});

describe('parseMarkdownSource — single file', () => {
  it('rejects a file with an unsupported extension', async () => {
    const file = path.join(dir, 'note.txt');
    await fs.writeFile(file, 'hello');
    await expect(parseMarkdownSource(file)).rejects.toThrow(/not a markdown file/);
  });

  it('extracts a title from a leading heading and strips it from the body', async () => {
    const file = path.join(dir, 'note.md');
    await fs.writeFile(file, '# My Title\n\nSome body text.');
    const result = await parseMarkdownSource(file);

    expect(result.notes).toHaveLength(1);
    expect(result.notes[0].title).toBe('My Title');
    expect(result.notes[0].body).toBe('Some body text.');
    expect(result.notes[0].notebookRef).toBe(ROOT_NOTEBOOK_REF);
    expect(result.notebooks).toHaveLength(0);
  });

  it('falls back to the filename when there is no heading or usable first line', async () => {
    const file = path.join(dir, 'my_note-file.md');
    await fs.writeFile(file, '\n\n');
    const result = await parseMarkdownSource(file);
    expect(result.notes[0].title).toBe('my note file');
  });

  it('reads frontmatter title/tags/notebook overrides', async () => {
    const file = path.join(dir, 'note.md');
    await fs.writeFile(
      file,
      ['---', 'title: Frontmatter Title', 'tags: [work, urgent]', 'notebook: Custom / Nested', '---', 'Body content.'].join('\n')
    );
    const result = await parseMarkdownSource(file);

    expect(result.notes[0].title).toBe('Frontmatter Title');
    expect(result.notes[0].tagRefs).toEqual(['work', 'urgent']);
    expect(result.notes[0].notebookRef).toBe('Custom/Nested');
    expect(result.notebooks.map((n) => n.ref)).toEqual(['Custom', 'Custom/Nested']);
    expect(result.notebooks.find((n) => n.ref === 'Custom/Nested')?.parentRef).toBe('Custom');
    expect(result.tags).toEqual([
      { ref: 'work', title: 'work' },
      { ref: 'urgent', title: 'urgent' },
    ]);
  });

  it('parses is_todo/todo_completed frontmatter as booleans', async () => {
    const file = path.join(dir, 'note.md');
    await fs.writeFile(file, '---\ntodo: true\ncompleted: true\n---\nDo the thing');
    const result = await parseMarkdownSource(file);
    expect(result.notes[0].isTodo).toBe(true);
    expect(result.notes[0].todoCompleted).toBe(true);
  });
});

describe('parseMarkdownSource — directory', () => {
  it('derives notebook structure from subdirectories', async () => {
    await fs.mkdir(path.join(dir, 'Projects', 'Alpha'), { recursive: true });
    await fs.writeFile(path.join(dir, 'root-note.md'), '# Root Note\n\nAt the top.');
    await fs.writeFile(path.join(dir, 'Projects', 'overview.md'), '# Overview\n\nProjects overview.');
    await fs.writeFile(path.join(dir, 'Projects', 'Alpha', 'plan.md'), '# Plan\n\nAlpha plan.');

    const result = await parseMarkdownSource(dir);

    expect(result.notes).toHaveLength(3);
    const byTitle = Object.fromEntries(result.notes.map((n) => [n.title, n]));
    expect(byTitle['Root Note'].notebookRef).toBe(ROOT_NOTEBOOK_REF);
    expect(byTitle['Overview'].notebookRef).toBe('Projects');
    expect(byTitle['Plan'].notebookRef).toBe('Projects/Alpha');

    const refs = result.notebooks.map((n) => n.ref).sort();
    expect(refs).toEqual(['Projects', 'Projects/Alpha']);
    expect(result.notebooks.find((n) => n.ref === 'Projects')?.parentRef).toBeUndefined();
    expect(result.notebooks.find((n) => n.ref === 'Projects/Alpha')?.parentRef).toBe('Projects');
  });

  it('ignores dotfiles/dot-directories and non-markdown files', async () => {
    await fs.mkdir(path.join(dir, '.git'), { recursive: true });
    await fs.writeFile(path.join(dir, '.git', 'config.md'), '# Should be ignored');
    await fs.writeFile(path.join(dir, 'readme.txt'), 'not markdown');
    await fs.writeFile(path.join(dir, 'note.md'), '# Kept\n\nBody');

    const result = await parseMarkdownSource(dir);
    expect(result.notes).toHaveLength(1);
    expect(result.notes[0].title).toBe('Kept');
  });

  it('records the absolute source file path on each note', async () => {
    await fs.writeFile(path.join(dir, 'note.md'), '# Title\n\nBody');
    const result = await parseMarkdownSource(dir);
    expect(result.notes[0].sourceFilePath).toBe(path.resolve(path.join(dir, 'note.md')));
  });
});

describe('parseMarkdownSource — local asset links', () => {
  it('rewrites a relative link to a real sibling file into a :/<hash> token and registers it as a resource', async () => {
    await fs.writeFile(path.join(dir, 'diagram.png'), Buffer.from([0x89, 0x50, 0x4e, 0x47]));
    await fs.writeFile(path.join(dir, 'note.md'), '# Title\n\nSee ![diagram](./diagram.png) above.');

    const result = await parseMarkdownSource(dir);

    expect(result.resources).toHaveLength(1);
    const resource = result.resources[0];
    expect(resource.filename).toBe('diagram.png');
    expect(resource.mime).toBe('image/png');
    expect(resource.data).toEqual(Buffer.from([0x89, 0x50, 0x4e, 0x47]));
    expect(resource.id).toMatch(/^[0-9a-f]{32}$/);
    expect(result.notes[0].body).toBe(`See ![diagram](:/${resource.id}) above.`);
  });

  it('deduplicates a shared asset referenced by multiple notes into a single resource', async () => {
    await fs.writeFile(path.join(dir, 'logo.png'), Buffer.from([1, 2, 3]));
    await fs.writeFile(path.join(dir, 'a.md'), '# A\n\n![logo](./logo.png)');
    await fs.writeFile(path.join(dir, 'b.md'), '# B\n\n![logo](./logo.png)');

    const result = await parseMarkdownSource(dir);

    expect(result.resources).toHaveLength(1);
    const id = result.resources[0].id;
    const a = result.notes.find((n) => n.title === 'A')!;
    const b = result.notes.find((n) => n.title === 'B')!;
    expect(a.body).toBe(`![logo](:/${id})`);
    expect(b.body).toBe(`![logo](:/${id})`);
  });

  it('leaves a link to another imported note untouched, for importer.ts to rewrite once note IDs exist', async () => {
    await fs.writeFile(path.join(dir, 'a.md'), '# A\n\nSee [B](./b.md).');
    await fs.writeFile(path.join(dir, 'b.md'), '# B\n\nBody.');

    const result = await parseMarkdownSource(dir);

    const a = result.notes.find((n) => n.title === 'A')!;
    expect(a.body).toBe('See [B](./b.md).');
    expect(result.resources).toHaveLength(0);
  });

  it('leaves a link to a nonexistent file untouched and does not register a resource', async () => {
    await fs.writeFile(path.join(dir, 'note.md'), '# Title\n\nSee [missing](./nope.png).');

    const result = await parseMarkdownSource(dir);

    expect(result.notes[0].body).toBe('See [missing](./nope.png).');
    expect(result.resources).toHaveLength(0);
  });

  it('leaves external http(s) links and existing :/ links untouched', async () => {
    await fs.writeFile(path.join(dir, 'note.md'), '# Title\n\n[site](https://example.com) and [existing](:/abc123).');

    const result = await parseMarkdownSource(dir);

    expect(result.notes[0].body).toBe('[site](https://example.com) and [existing](:/abc123).');
    expect(result.resources).toHaveLength(0);
  });
});
