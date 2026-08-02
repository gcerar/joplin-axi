import type { JoplinClient } from '../src/client.js';
import type { ParsedArgs } from '../src/lib/args.js';
import type { CommandOutput } from '../src/lib/command.js';

export function args(positionals: string[] = [], flags: Record<string, string | boolean | number> = {}): ParsedArgs {
  return { positionals, flags, help: false };
}

/** A Command.run() result is either a bare string (exit 0) or {output, exitCode}. */
export function textOf(result: string | CommandOutput): string {
  return typeof result === 'string' ? result : result.output;
}

/** Exit code a Command.run() result implies — bare strings always mean 0. */
export function exitCodeOf(result: string | CommandOutput): number {
  return typeof result === 'string' ? 0 : result.exitCode;
}

/** Builds a fakeClient(overrides) factory pre-seeded with a test file's own defaults. */
export function fakeClientFactory(defaults: Partial<Record<keyof JoplinClient, unknown>>) {
  return (overrides: Partial<Record<keyof JoplinClient, unknown>> = {}): JoplinClient =>
    ({ ...defaults, ...overrides }) as unknown as JoplinClient;
}
