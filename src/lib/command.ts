import type { JoplinClient } from '../client.js';
import type { CommandSpec, ParsedArgs } from './args.js';

/**
 * Non-zero exitCode lets a command signal a real failure (e.g. a batch
 * mutation where at least one item failed) while still printing its full
 * report — throwing a UsageError/Error would suppress the output entirely.
 */
export interface CommandOutput {
  output: string;
  exitCode: number;
}

export interface Command {
  spec: CommandSpec;
  run: (parsed: ParsedArgs, client: JoplinClient) => Promise<string | CommandOutput>;
}
