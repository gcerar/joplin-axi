import { describe, expect, it } from 'vitest';
import { type CommandSpec, helpText, parseArgs, splitList, UsageError } from '../src/lib/args.js';

const spec: CommandSpec = {
  name: 'notes list',
  summary: 'List notes.',
  usage: 'joplin-axi notes list [--query <text>] [--limit <n>]',
  flags: {
    query: { type: 'string', description: 'Free-text search' },
    limit: { type: 'number', description: 'Max results', default: 20 },
    task: { type: 'boolean', description: 'Restrict to to-dos', default: false },
  },
  examples: ['joplin-axi notes list --task'],
};

describe('parseArgs', () => {
  it('applies defaults when a flag is omitted', () => {
    const parsed = parseArgs([], spec);
    expect(parsed.flags.limit).toBe(20);
    expect(parsed.flags.task).toBe(false);
  });

  it('parses --flag value pairs', () => {
    const parsed = parseArgs(['--query', 'annual report'], spec);
    expect(parsed.flags.query).toBe('annual report');
  });

  it('parses --flag=value form', () => {
    const parsed = parseArgs(['--limit=5'], spec);
    expect(parsed.flags.limit).toBe(5);
  });

  it('rejects a non-numeric value for a number flag instead of producing NaN', () => {
    expect(() => parseArgs(['--limit', 'abc'], spec)).toThrow(UsageError);
    expect(() => parseArgs(['--limit=abc'], spec)).toThrow(UsageError);
  });

  it('treats a boolean flag with no value as true', () => {
    const parsed = parseArgs(['--task'], spec);
    expect(parsed.flags.task).toBe(true);
  });

  it('parses --flag=true/--flag=false explicitly', () => {
    expect(parseArgs(['--task=true'], spec).flags.task).toBe(true);
    expect(parseArgs(['--task=false'], spec).flags.task).toBe(false);
  });

  it('rejects a non-boolean value for a boolean flag instead of silently treating it as true', () => {
    expect(() => parseArgs(['--task=0'], spec)).toThrow(UsageError);
    expect(() => parseArgs(['--task=no'], spec)).toThrow(UsageError);
  });

  it('collects positionals separately from flags', () => {
    const parsed = parseArgs(['abc123', '--limit', '5'], spec);
    expect(parsed.positionals).toEqual(['abc123']);
    expect(parsed.flags.limit).toBe(5);
  });

  it('sets help and stops treating it as a positional', () => {
    const parsed = parseArgs(['--help'], spec);
    expect(parsed.help).toBe(true);
    expect(parsed.positionals).toEqual([]);
  });

  it('throws UsageError on an unknown flag, listing the valid ones', () => {
    expect(() => parseArgs(['--bogus'], spec)).toThrow(UsageError);
    try {
      parseArgs(['--bogus'], spec);
    } catch (e) {
      expect((e as Error).message).toContain('unknown flag --bogus');
      expect((e as UsageError).helpLines.join(' ')).toContain('--query');
    }
  });

  it('throws UsageError when a string flag is missing its value', () => {
    expect(() => parseArgs(['--query'], spec)).toThrow(UsageError);
  });

  it('lets --help short-circuit past a later invalid flag', () => {
    // notes list --help --bogus: previously kept validating after spotting
    // --help and threw on --bogus instead of showing help.
    expect(() => parseArgs(['--help', '--bogus'], spec)).not.toThrow();
    expect(parseArgs(['--help', '--bogus'], spec).help).toBe(true);
  });

  it('does not treat a flag VALUE of "--help" as a help request', () => {
    // --query is a string flag, so the token right after it is consumed as
    // its value — "--help" here should become the query text, not trigger help.
    const parsed = parseArgs(['--query', '--help'], spec);
    expect(parsed.flags.query).toBe('--help');
    expect(parsed.help).toBe(false);
  });
});

describe('splitList', () => {
  it('splits, trims, and drops empty entries', () => {
    expect(splitList('a, b ,, c')).toEqual(['a', 'b', 'c']);
  });

  it('returns an empty array for an empty string', () => {
    expect(splitList('')).toEqual([]);
  });
});

describe('helpText', () => {
  it('includes usage, flags with defaults, and examples', () => {
    const text = helpText(spec);
    expect(text).toContain('usage: joplin-axi notes list');
    expect(text).toContain('--limit <number>');
    expect(text).toContain('(default: 20)');
    expect(text).toContain('joplin-axi notes list --task');
  });
});
