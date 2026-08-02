// Minimal TOON (Token-Oriented Object Notation) writer.
// Spec: https://toonformat.dev — this covers the subset AXI tools need:
// tables, scalars, objects, help hints, and structured errors.

/** Formats a Joplin epoch-ms timestamp field, shared by every command that displays one. */
export function fmtTime(ms: number | undefined): string {
  return ms ? new Date(ms).toISOString() : '';
}

function quoteIfNeeded(value: string): string {
  if (/[",\n]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

function cell(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return quoteIfNeeded(value);
  return String(value);
}

export function table(name: string, fields: string[], rows: Record<string, unknown>[]): string {
  const header = `${name}[${rows.length}]{${fields.join(',')}}:`;
  const lines = rows.map((row) => '  ' + fields.map((f) => cell(row[f])).join(','));
  return [header, ...lines].join('\n');
}

export function scalar(key: string, value: unknown): string {
  return `${key}: ${value}`;
}

function indentMultiline(value: string): string {
  return value
    .split('\n')
    .map((line, i) => (i === 0 ? line : '    ' + line))
    .join('\n');
}

export function object(name: string, obj: Record<string, unknown>): string {
  const lines = Object.entries(obj).map(([k, v]) => `  ${k}: ${indentMultiline(String(v))}`);
  return [`${name}:`, ...lines].join('\n');
}

export function help(lines: string[]): string {
  if (lines.length === 0) return '';
  return [`help[${lines.length}]:`, ...lines.map((l) => '  ' + l)].join('\n');
}

export function errorOut(message: string, helpLines: string[] = []): string {
  const parts = [`error: ${message}`, ...helpLines.map((l) => `help: ${l}`)];
  return parts.join('\n');
}

export function truncate(text: string, limit = 800): { text: string; truncated: boolean; total: number } {
  const total = text.length;
  if (total <= limit) return { text, truncated: false, total };
  return { text: text.slice(0, limit), truncated: true, total };
}

export function sections(...parts: string[]): string {
  return parts.filter((p) => p.length > 0).join('\n\n');
}
