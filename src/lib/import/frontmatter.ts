// Minimal `---\nkey: value\n---` frontmatter parser for markdown import. No
// YAML dependency (matching this project's "no dependency, full control"
// style — see src/lib/args.ts) — only handles the small field shapes markdown
// import actually needs: flat scalars, `[a, b]`/comma-separated inline lists,
// and `- item` block lists. Anything fancier (nested maps, multiline scalars,
// YAML anchors) is out of scope; such a file just gets treated as having no
// frontmatter (the `---` block, if any, is left in the body untouched).

export interface ParsedFrontmatter {
  fields: Record<string, string>;
  lists: Record<string, string[]>;
  body: string;
}

const DELIMITER_RE = /^---\s*$/;

function stripQuotes(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length >= 2 && ((trimmed[0] === '"' && trimmed.endsWith('"')) || (trimmed[0] === "'" && trimmed.endsWith("'")))) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

function splitInlineList(value: string): string[] {
  const inner = value.trim().replace(/^\[/, '').replace(/\]$/, '');
  return inner
    .split(',')
    .map((s) => stripQuotes(s))
    .filter(Boolean);
}

// Keys treated as comma-separated lists when written as a bare scalar (no
// brackets) — e.g. `tags: work, urgent`. Deliberately NOT a generic
// "any comma-containing value is a list" rule: a `title: Hello, World` would
// wrongly split otherwise. Bracket syntax (`key: [a, b]`) is unambiguous and
// applies to any key regardless of this list.
const DEFAULT_LIST_KEYS = ['tags', 'categories', 'keywords'];

export function parseFrontmatter(content: string, listKeys: string[] = DEFAULT_LIST_KEYS): ParsedFrontmatter {
  const noFrontmatter = { fields: {}, lists: {}, body: content };
  if (!DELIMITER_RE.test(content.split(/\r?\n/, 1)[0] ?? '')) return noFrontmatter;

  const lines = content.split(/\r?\n/);
  let end = -1;
  for (let i = 1; i < lines.length; i++) {
    if (DELIMITER_RE.test(lines[i])) {
      end = i;
      break;
    }
  }
  if (end === -1) return noFrontmatter;

  const fields: Record<string, string> = {};
  const lists: Record<string, string[]> = {};

  let i = 1;
  while (i < end) {
    const line = lines[i];
    const m = /^([A-Za-z0-9_-]+):\s*(.*)$/.exec(line);
    if (!m) {
      i++;
      continue;
    }
    const [, key, rawValue] = m;

    if (rawValue.trim().startsWith('[')) {
      lists[key] = splitInlineList(rawValue);
      i++;
      continue;
    }

    if (rawValue.trim() === '') {
      // Possible YAML block list on following indented `- item` lines.
      const items: string[] = [];
      let j = i + 1;
      while (j < end && /^\s*-\s*/.test(lines[j])) {
        items.push(stripQuotes(lines[j].replace(/^\s*-\s*/, '')));
        j++;
      }
      if (items.length) {
        lists[key] = items;
        i = j;
        continue;
      }
      fields[key] = '';
      i++;
      continue;
    }

    if (listKeys.includes(key) && rawValue.includes(',')) {
      // A bare comma-separated list scalar (common for `tags: a, b, c` without brackets).
      lists[key] = rawValue.split(',').map(stripQuotes).filter(Boolean);
      i++;
      continue;
    }

    fields[key] = stripQuotes(rawValue);
    i++;
  }

  const body = lines
    .slice(end + 1)
    .join('\n')
    .replace(/^\n+/, '');
  return { fields, lists, body };
}
