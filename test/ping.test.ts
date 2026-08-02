import { describe, expect, it, vi } from 'vitest';
import { pingCommand } from '../src/commands/ping.js';
import { args, fakeClientFactory } from './helpers.js';

const NO_ARGS = args();

const fakeClient = fakeClientFactory({
  ping: vi.fn().mockResolvedValue(true),
  listNotebooks: vi.fn().mockResolvedValue([]),
});

describe('ping', () => {
  it('reports unreachable and failed auth without attempting an authenticated call', async () => {
    const listNotebooks = vi.fn();
    const client = fakeClient({ ping: vi.fn().mockResolvedValue(false), listNotebooks });

    const out = await pingCommand.run(NO_ARGS, client);

    expect(out).toContain('clipper: unreachable');
    expect(out).toContain('auth: failed');
    expect(listNotebooks).not.toHaveBeenCalled();
  });

  it('reports reachable and auth ok when an authenticated call succeeds', async () => {
    const client = fakeClient({ listNotebooks: vi.fn().mockResolvedValue([]) });

    const out = await pingCommand.run(NO_ARGS, client);

    expect(out).toContain('clipper: reachable');
    expect(out).toContain('auth: ok');
  });

  it('reports reachable but failed auth when the token is rejected', async () => {
    const client = fakeClient({ listNotebooks: vi.fn().mockRejectedValue(new Error('401')) });

    const out = await pingCommand.run(NO_ARGS, client);

    expect(out).toContain('clipper: reachable');
    expect(out).toContain('auth: failed');
  });
});
