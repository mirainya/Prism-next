import { request } from './request';
import {
  clearUnifiedCredentialRouteStates,
  type UnifiedCredential,
  type UnifiedGatewayPage,
} from './unifiedGatewayApi';

export type CredentialPurpose =
  | 'execution'
  | 'catalog_discovery'
  | 'availability_read'
  | 'upstream_callback_verify';
export interface CredentialFields {
  request_limit: number | null;
  task_limit: number | null;
  weight: number;
}
export interface CredentialUpdateFields extends CredentialFields {
  secret?: string;
}
export interface CredentialCreateFields extends CredentialFields {
  credential_code: string;
  secret: string;
  purposes: CredentialPurpose[];
}
export interface CredentialCreateResult {
  id: number;
  pool_id: number;
}
export type CredentialTransition = {
  status: 'draining' | 'disabled';
  label: '停止使用' | '完成停用';
};
export const managedCredentialTransition = (
  status: string,
): CredentialTransition | null => {
  if (status === 'active') return { status: 'draining', label: '停止使用' };
  if (status === 'draining') return { status: 'disabled', label: '完成停用' };
  return null;
};
export const fetchManagedCredentials = (
  page: number,
  size: number,
  poolId?: number,
  signal?: AbortSignal,
) => {
  const params = new URLSearchParams({
    page: String(page),
    page_size: String(size),
  });
  if (poolId) params.set('pool_id', String(poolId));
  return request<UnifiedGatewayPage<UnifiedCredential>>(
    `/admin/unified-gateway/credentials?${params}`,
    { signal },
  );
};
export const createManagedCredential = (
  poolId: number,
  data: CredentialCreateFields,
) =>
  request<{ id: number }>(`/admin/unified-gateway/pools/${poolId}/credentials`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
// Sharing an existing pool is opt-in. Never fall back to it after an isolated
// creation fails: that would silently inherit different prices and permissions.
export const createManagedCredentialForChannel = async (
  channelId: number,
  data: CredentialCreateFields,
  sharedPoolId?: number,
): Promise<CredentialCreateResult> => {
  if (sharedPoolId !== undefined) {
    const created = await createManagedCredential(sharedPoolId, data);
    return { ...created, pool_id: sharedPoolId };
  }
  return request<CredentialCreateResult>(`/admin/unified-gateway/channels/${channelId}/credentials`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
};
export const updateManagedCredential = async (
  item: UnifiedCredential,
  data: CredentialUpdateFields,
) => {
  const result = await request(`/admin/unified-gateway/credentials/${item.id}`, {
    method: 'PUT',
    body: JSON.stringify({ ...data, expected_version: item.config_version }),
  });
  if (data.secret) await clearUnifiedCredentialRouteStates(item.id);
  return result;
};
export const transitionManagedCredential = (item: UnifiedCredential) => {
  const transition = managedCredentialTransition(item.status);
  if (!transition) return Promise.reject(new Error('此 API Key 已停用'));
  return request(`/admin/unified-gateway/credentials/${item.id}/status`, {
    method: 'POST',
    body: JSON.stringify({
      status: transition.status,
      expected_version: item.config_version,
    }),
  });
};
export interface CredentialRemovalImpact {
  credential_id: number;
  config_version: number;
  status: string;
  secret_removed: boolean;
  active_slots: number;
  unresolved_attempts: number;
  control_runs: number;
  pending_deliveries: number;
  other_usable_keys: number;
  models: { api_name: string; unavailable: boolean }[];
}
export const fetchCredentialRemovalImpact = (item: UnifiedCredential) =>
  request<CredentialRemovalImpact>(`/admin/unified-gateway/credentials/${item.id}/removal-impact`);
export const removeManagedCredential = (item: UnifiedCredential) =>
  request<CredentialRemovalImpact>(`/admin/unified-gateway/credentials/${item.id}/remove`, {
    method: 'POST',
    body: JSON.stringify({ expected_version: item.config_version }),
  });
