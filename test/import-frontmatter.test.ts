import { describe, expect, it } from 'vitest';
import { parseFrontmatter } from '../src/lib/import/frontmatter.js';

describe('parseFrontmatter', () => {
  it('returns no frontmatter and the whole content as body when there is no leading --- block', () => {
    const result = parseFrontmatter('# Just a note\n\nbody text');
    expect(result.fields).toEqual({});
    expect(result.lists).toEqual({});
    expect(result.body).toBe('# Just a note\n\nbody text');
  });

  it('parses flat scalar fields', () => {
    const result = parseFrontmatter('---\ntitle: Hello World\nnotebook: Work / Projects\n---\nBody here');
    expect(result.fields).toEqual({ title: 'Hello World', notebook: 'Work / Projects' });
    expect(result.body).toBe('Body here');
  });

  it('strips surrounding quotes from scalar values', () => {
    const result = parseFrontmatter('---\ntitle: "Quoted Title"\n---\nBody');
    expect(result.fields.title).toBe('Quoted Title');
  });

  it('parses a bracket inline list for any key', () => {
    const result = parseFrontmatter('---\ntags: [work, urgent]\n---\nBody');
    expect(result.lists.tags).toEqual(['work', 'urgent']);
  });

  it('parses a bare comma-separated list only for known list keys, not an arbitrary scalar', () => {
    const result = parseFrontmatter('---\ntags: work, urgent\ntitle: Hello, World\n---\nBody');
    expect(result.lists.tags).toEqual(['work', 'urgent']);
    // "title" isn't a list key, so its comma must NOT cause a split —
    // this is the regression case the DEFAULT_LIST_KEYS scoping exists for.
    expect(result.fields.title).toBe('Hello, World');
    expect(result.lists.title).toBeUndefined();
  });

  it('parses a YAML block list', () => {
    const result = parseFrontmatter('---\ntags:\n  - work\n  - urgent\n---\nBody');
    expect(result.lists.tags).toEqual(['work', 'urgent']);
  });

  it('leaves the body untouched if the closing delimiter is never found', () => {
    const content = '---\ntitle: Hello\nno closing delimiter here';
    const result = parseFrontmatter(content);
    expect(result.fields).toEqual({});
    expect(result.body).toBe(content);
  });

  it('trims leading blank lines from the body after the closing delimiter', () => {
    const result = parseFrontmatter('---\ntitle: Hello\n---\n\n\nActual body');
    expect(result.body).toBe('Actual body');
  });
});
