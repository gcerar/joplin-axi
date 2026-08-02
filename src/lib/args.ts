// Minimal flag parser. No dependency, full control over AXI-style usage errors
// (exit code 2, unknown flags rejected with the valid set inlined).

export class UsageError extends Error {
  helpLines: string[];

  constructor(message: string, helpLines: string[] = []) {
    super(message);
    this.helpLines = helpLines;
  }
}

export interface FlagSpec {
  type: 'string' | 'boolean' | 'number';
  description: string;
  default?: string | boolean | number;
}

export interface CommandSpec {
  name: string;
  summary: string;
  usage: string;
  flags: Record<string, FlagSpec>;
  examples: string[];
}

export interface ParsedArgs {
  positionals: string[];
  flags: Record<string, string | boolean | number>;
  help: boolean;
}

export function parseArgs(argv: string[], spec: CommandSpec): ParsedArgs {
  const flags: Record<string, string | boolean | number> = {};
  const positionals: string[] = [];

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];

    // Short-circuits rather than just setting a flag and continuing, so a bare
    // --help always wins over flag validation later in the same argv — e.g.
    // `notes list --help --bogus` shows help instead of erroring on --bogus.
    // (A token consumed as a preceding string flag's *value* never reaches this
    // check, so `--title --help` correctly treats "--help" as the title, not a
    // help request — see args.test.ts.)
    if (arg === '--help' || arg === '-h') {
      return { positionals, flags, help: true };
    }

    if (arg.startsWith('--')) {
      const eq = arg.indexOf('=');
      const name = eq === -1 ? arg.slice(2) : arg.slice(2, eq);
      const flagSpec = spec.flags[name];
      if (!flagSpec) {
        const valid = Object.keys(spec.flags).map((f) => `--${f}`);
        throw new UsageError(`unknown flag --${name} for \`${spec.name}\``, [
          `valid flags for \`${spec.name}\`: ${valid.length ? valid.join(', ') : '(none)'}`,
        ]);
      }

      if (flagSpec.type === 'boolean') {
        if (eq === -1) {
          flags[name] = true;
        } else {
          const value = arg.slice(eq + 1);
          if (value !== 'true' && value !== 'false') {
            throw new UsageError(`--${name} must be \`true\` or \`false\`, got \`${value}\``, [spec.usage]);
          }
          flags[name] = value === 'true';
        }
        continue;
      }

      let value: string;
      if (eq !== -1) {
        value = arg.slice(eq + 1);
      } else {
        const next = argv[++i];
        if (next === undefined) {
          throw new UsageError(`--${name} requires a value`, [spec.usage]);
        }
        value = next;
      }
      if (flagSpec.type === 'number') {
        const num = Number(value);
        if (Number.isNaN(num)) {
          throw new UsageError(`--${name} must be a number, got \`${value}\``, [spec.usage]);
        }
        flags[name] = num;
      } else {
        flags[name] = value;
      }
      continue;
    }

    positionals.push(arg);
  }

  for (const [name, flagSpec] of Object.entries(spec.flags)) {
    if (!(name in flags) && flagSpec.default !== undefined) {
      flags[name] = flagSpec.default;
    }
  }

  return { positionals, flags, help: false };
}

/** Comma-separated flag value → trimmed, non-empty entries. Shared by every
 * command that takes a `--fields`/`--notes`-style list flag. */
export function splitList(raw: unknown): string[] {
  return String(raw)
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

/** Fetches positional[index], throwing a consistent UsageError if missing.
 * Shared by every command with a required `<id>`/`<title>`-style argument. */
export function requirePositional(parsed: ParsedArgs, index: number, name: string, usage: string): string {
  const value = parsed.positionals[index];
  if (!value) throw new UsageError(`missing required argument <${name}>`, [usage]);
  return value;
}

export function helpText(spec: CommandSpec): string {
  const lines = [spec.summary, '', `usage: ${spec.usage}`];

  const flagEntries = Object.entries(spec.flags);
  if (flagEntries.length) {
    lines.push('', 'flags:');
    for (const [name, f] of flagEntries) {
      const def = f.default !== undefined ? ` (default: ${f.default})` : '';
      lines.push(`  --${name} <${f.type}>  ${f.description}${def}`);
    }
  }

  if (spec.examples.length) {
    lines.push('', 'examples:');
    for (const ex of spec.examples) lines.push(`  ${ex}`);
  }

  return lines.join('\n');
}
