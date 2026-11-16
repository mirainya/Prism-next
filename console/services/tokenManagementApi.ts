import { ApiToken } from '../types';
import { request } from './request';

export interface ManagedToken extends Omit<ApiToken, 'status'> {
  status: 'active' | 'paused' | 'expired' | 'revoked';
  userID: string;
  username: string;
  ownerEnabled: boolean;
  authVersion: string;
  rateLimit: number;
  createdAt: string;
  expiresAt: string | null;
  revokedAt: string | null;
}
export interface TokenAccessEvent {
  id: number; auth_version: string; old_status: number; new_status: number;
  reason_code: string; actor_user_id: number; created_at: string; note: string;
}
export interface ManagementQuery { page: number; page_size: number; user_id?: string; token_id?: string; search?: string; state?: string }
interface Page<T> { items: T[]; total: number; page: number; page_size: number }

export async function fetchManagedTokens(query: ManagementQuery, signal?: AbortSignal): Promise<Page<ManagedToken>> {
  const params = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => { if (value !== undefined && value !== '') params.set(key, String(value)); });
  const result = await request<Page<any>>(`/tokens/management?${params}`, { signal });
  return { ...result, items: result.items.map(token => ({
    id: String(token.id), name: token.name, key: token.key_hint, balance: 0, totalUsed: 0,
    budget: token.budget, status: token.state, userID: String(token.user_id), username: token.username,
    ownerEnabled: token.owner_enabled, authVersion: token.auth_version, rateLimit: token.rate_limit, createdAt: token.created_at,
    expiresAt: token.expires_at, revokedAt: token.revoked_at,
    xfsStorage: { configured: token.xfs_storage?.configured === true, keyHint: token.xfs_storage?.key_hint || '', baseURL: token.xfs_storage?.base_url || '' },
  })) };
}

export const changeTokenAccess = (token: ManagedToken, action: 'pause' | 'resume' | 'revoke', note: string) =>
  request(`/tokens/${token.id}/access`, { method: 'POST', body: JSON.stringify({ action, expected_version: token.authVersion, note }) });

export const fetchTokenAccessHistory = (id: string, page: number, signal?: AbortSignal): Promise<Page<TokenAccessEvent>> =>
  request(`/tokens/${id}/access-history?page=${page}&page_size=10`, { signal });

export interface TokenRateSettings { enabled: boolean; requests_per_min: number }
export const fetchTokenRateSettings = (): Promise<TokenRateSettings> => request('/tokens/rate-limit-settings');

export const updateTokenPolicy = (token: ManagedToken, rateLimit: number, expiresAt: string | null, note: string) =>
  request(`/tokens/${token.id}/policy`, { method: 'PUT', body: JSON.stringify({ expected_version: token.authVersion, rate_limit: rateLimit, expires_at: expiresAt, note }) });
