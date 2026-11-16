/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';

vi.mock('node:fs', () => ({ readFileSync: vi.fn() }));

/** An unsigned JWT-shaped token expiring `seconds` from now; only `exp` is read. */
function token(seconds: number): string {
  const payload = Buffer.from(
    JSON.stringify({ exp: Math.floor(Date.now() / 1000) + seconds })
  ).toString('base64url');
  return `eyJhbGciOiJub25lIn0.${payload}.sig`;
}

function seedAccount(accessToken: string) {
  vi.mocked(readFileSync).mockReturnValue(
    JSON.stringify({ email: 'owner@example.test', password: 'pw', accessToken })
  );
}

function loginAnswers(accessToken: string) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ success: true, data: { access_token: accessToken } }), {
      headers: { 'Content-Type': 'application/json' },
    })
  );
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('ownerToken', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it('reuses the globalSetup token while it is valid', async () => {
    const fresh = token(900);
    seedAccount(fresh);
    const fetchMock = loginAnswers('unused');
    const { ownerToken } = await import('./api');

    expect(await ownerToken()).toBe(fresh);
    expect(await ownerToken()).toBe(fresh);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('logs in again once the token is about to expire', async () => {
    seedAccount(token(30));
    const renewed = token(900);
    const fetchMock = loginAnswers(renewed);
    const { ownerToken } = await import('./api');

    expect(await ownerToken()).toBe(renewed);
    expect(await ownerToken()).toBe(renewed);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/auth\/login$/);
  });
});
