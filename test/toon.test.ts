import { describe, expect, it } from 'vitest';
import { errorOut, help, object, scalar, sections, table, truncate } from '../src/toon.js';

describe('table', () => {
  it('renders header with count and fields', () => {
    const out = table('notes', ['id', 'title'], [{ id: '1', title: 'Fix bug' }]);
    expect(out).toBe('notes[1]{id,title}:\n  1,Fix bug');
  });

  it('renders an empty table with zero count', () => {
    const out = table('notes', ['id', 'title'], []);
    expect(out).toBe('notes[0]{id,title}:');
  });

  it('quotes values containing commas or quotes', () => {
    const out = table('notes', ['title'], [{ title: 'a, "b"' }]);
    expect(out).toBe('notes[1]{title}:\n  "a, ""b"""');
  });
});

describe('scalar', () => {
  it('renders key: value', () => {
    expect(scalar('count', '30 of 847 total')).toBe('count: 30 of 847 total');
  });
});

describe('object', () => {
  it('renders nested key: value lines', () => {
    const out = object('note', { id: '42', title: 'Example' });
    expect(out).toBe('note:\n  id: 42\n  title: Example');
  });
});

describe('help', () => {
  it('renders numbered help block', () => {
    expect(help(['do this', 'do that'])).toBe('help[2]:\n  do this\n  do that');
  });

  it('renders nothing for an empty list', () => {
    expect(help([])).toBe('');
  });
});

describe('errorOut', () => {
  it('renders error and help lines', () => {
    expect(errorOut('bad flag', ['try --help'])).toBe('error: bad flag\nhelp: try --help');
  });
});

describe('truncate', () => {
  it('passes short text through untouched', () => {
    expect(truncate('short', 10)).toEqual({ text: 'short', truncated: false, total: 5 });
  });

  it('truncates long text and reports total length', () => {
    const result = truncate('0123456789', 5);
    expect(result).toEqual({ text: '01234', truncated: true, total: 10 });
  });
});

describe('sections', () => {
  it('joins non-empty parts with blank lines and drops empty ones', () => {
    expect(sections('a', '', 'b')).toBe('a\n\nb');
  });
});
