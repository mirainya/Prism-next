import { afterEach, describe, expect, it, vi } from 'vitest';
import { expirationISO, toLocalExpiration, validTokenPolicy } from './TokenPolicyFields';

afterEach(() => vi.useRealTimers());
describe('Key policy input', () => {
  it('requires a bounded whole number and a future expiration', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-11-07T00:00:00Z'));
    expect(validTokenPolicy('0', '')).toBe(true);
    expect(validTokenPolicy('1000000', '2026-11-08T00:00:00Z')).toBe(true);
    for (const value of ['', '-1', '1.5', '1e3', '1000001']) expect(validTokenPolicy(value, '')).toBe(false);
    for (const value of ['bad date', '2026-01-01T00:00:00Z', '2026-11-07T00:00:00Z']) expect(validTokenPolicy('60', value)).toBe(false);
  });
  it('converts local input to UTC without changing the instant', () => {
    const expiration = '2027-04-15T05:34:56.000Z';
    expect(expirationISO(toLocalExpiration(expiration))).toBe(expiration);
    expect(expirationISO('')).toBeNull();
    expect(toLocalExpiration(null)).toBe('');
  });
});
