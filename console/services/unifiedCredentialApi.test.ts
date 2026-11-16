import { afterEach, expect, it, vi } from 'vitest';
import { request } from './request';
import {
  createManagedCredential,
  createManagedCredentialForChannel,
  fetchManagedCredentials,
  managedCredentialTransition,
  transitionManagedCredential,
  updateManagedCredential,
  removeManagedCredential,
  fetchCredentialRemovalImpact,
} from './unifiedCredentialApi';
import {
  clearUnifiedCredentialRouteStates,
  type UnifiedCredential,
} from './unifiedGatewayApi';

vi.mock('./request', () => ({ request: vi.fn().mockResolvedValue({}) }));
vi.mock('./unifiedGatewayApi', () => ({
  clearUnifiedCredentialRouteStates: vi.fn().mockResolvedValue({ cleared: 0 }),
}));
afterEach(() => vi.clearAllMocks());
it('creates an isolated key by default and returns its new pool', async () => {
  const fields = {
    credential_code: 'independent', secret: 'test-only-secret',
    purposes: ['execution'] as const, weight: 1, request_limit: null, task_limit: null,
  };
  vi.mocked(request).mockResolvedValueOnce({ id: 12, pool_id: 15 });
  const result = await createManagedCredentialForChannel(7, { ...fields, purposes: [...fields.purposes] });
  expect(result).toEqual({ id: 12, pool_id: 15 });
  expect(request).toHaveBeenCalledExactlyOnceWith('/admin/unified-gateway/channels/7/credentials', {
    method: 'POST', body: JSON.stringify(fields),
  });
});
it('shares a pool only when its id is explicitly supplied', async () => {
  const fields = {
    credential_code: 'shared', secret: 'test-only-secret',
    purposes: ['execution'] as const, weight: 1, request_limit: null, task_limit: null,
  };
  vi.mocked(request).mockResolvedValueOnce({ id: 12 });
  const result = await createManagedCredentialForChannel(7, { ...fields, purposes: [...fields.purposes] }, 8);
  expect(result).toEqual({ id: 12, pool_id: 8 });
  expect(request).toHaveBeenCalledExactlyOnceWith('/admin/unified-gateway/pools/8/credentials', {
    method: 'POST', body: JSON.stringify(fields),
  });
});
it('does not retry isolated creation in a shared pool after an error', async () => {
  vi.mocked(request).mockRejectedValueOnce(new Error('duplicate key'));
  await expect(createManagedCredentialForChannel(7, {
    credential_code: 'independent', secret: 'test-only-secret', purposes: ['execution'],
    weight: 1, request_limit: null, task_limit: null,
  })).rejects.toThrow('duplicate key');
  expect(request).toHaveBeenCalledTimes(1);
});
it('filters the selected pool and cancels stale requests', async () => {
  const controller = new AbortController();
  await fetchManagedCredentials(2, 50, 8, controller.signal);
  expect(request).toHaveBeenCalledWith(
    '/admin/unified-gateway/credentials?page=2&page_size=50&pool_id=8',
    { signal: controller.signal },
  );
});
it('creates and updates credentials with the current config version', async () => {
  const fields = { weight: 3, request_limit: null, task_limit: 2 };
  await createManagedCredential(8, {
    ...fields,
    credential_code: 'test',
    secret: 'test-only-secret',
    purposes: ['execution'],
  });
  expect(request).toHaveBeenLastCalledWith(
    '/admin/unified-gateway/pools/8/credentials',
    {
      method: 'POST',
      body: JSON.stringify({
        ...fields,
        credential_code: 'test',
        secret: 'test-only-secret',
        purposes: ['execution'],
      }),
    },
  );
  const item = {
    id: 5,
    config_version: 7,
    status: 'active',
  } as UnifiedCredential;
  await updateManagedCredential(item, fields);
  expect(request).toHaveBeenLastCalledWith(
    '/admin/unified-gateway/credentials/5',
    { method: 'PUT', body: JSON.stringify({ ...fields, expected_version: 7 }) },
  );
  expect(clearUnifiedCredentialRouteStates).not.toHaveBeenCalled();
});

it('clears existing route breakers after replacing a key', async () => {
  const item = {
    id: 5,
    config_version: 7,
    status: 'active',
  } as UnifiedCredential;
  await updateManagedCredential(item, {
    weight: 3,
    request_limit: null,
    task_limit: 2,
    secret: 'replacement-secret',
  });
  expect(request).toHaveBeenCalledWith(
    '/admin/unified-gateway/credentials/5',
    {
      method: 'PUT',
      body: JSON.stringify({
        weight: 3,
        request_limit: null,
        task_limit: 2,
        secret: 'replacement-secret',
        expected_version: 7,
      }),
    },
  );
  expect(clearUnifiedCredentialRouteStates).toHaveBeenCalledWith(5);
  expect(vi.mocked(request).mock.invocationCallOrder[0]).toBeLessThan(
    vi.mocked(clearUnifiedCredentialRouteStates).mock.invocationCallOrder[0],
  );
});

it('does not clear route breakers when key replacement fails', async () => {
  vi.mocked(request).mockRejectedValueOnce(new Error('version conflict'));
  await expect(updateManagedCredential({
    id: 5,
    config_version: 7,
    status: 'active',
  } as UnifiedCredential, {
    weight: 3,
    request_limit: null,
    task_limit: 2,
    secret: 'replacement-secret',
  })).rejects.toThrow('version conflict');
  expect(clearUnifiedCredentialRouteStates).not.toHaveBeenCalled();
});

it('uses explicit active and draining lifecycle actions', async () => {
  const active = {
    id: 5,
    config_version: 7,
    status: 'active',
  } as UnifiedCredential;
  expect(managedCredentialTransition(active.status)).toEqual({
    status: 'draining',
    label: '停止使用',
  });
  await transitionManagedCredential(active);
  expect(request).toHaveBeenLastCalledWith(
    '/admin/unified-gateway/credentials/5/status',
    {
      method: 'POST',
      body: JSON.stringify({ status: 'draining', expected_version: 7 }),
    },
  );

  const draining = { ...active, status: 'draining', config_version: 8 };
  expect(managedCredentialTransition(draining.status)).toEqual({
    status: 'disabled',
    label: '完成停用',
  });
  await transitionManagedCredential(draining);
  expect(request).toHaveBeenLastCalledWith(
    '/admin/unified-gateway/credentials/5/status',
    {
      method: 'POST',
      body: JSON.stringify({ status: 'disabled', expected_version: 8 }),
    },
  );
  expect(managedCredentialTransition('disabled')).toBeNull();
  vi.mocked(request).mockClear();
  await expect(transitionManagedCredential({
    ...draining,
    status: 'disabled',
  })).rejects.toThrow('此 API Key 已停用');
  expect(request).not.toHaveBeenCalled();
});

it('removes a key with its current version and keeps server impact details', async () => {
  const item = { id: 5, config_version: 9, status: 'draining' } as UnifiedCredential;
  vi.mocked(request).mockResolvedValueOnce({ credential_id: 5, secret_removed: false, pending_deliveries: 1 });
  await expect(removeManagedCredential(item)).resolves.toMatchObject({ secret_removed: false });
  expect(request).toHaveBeenCalledWith('/admin/unified-gateway/credentials/5/remove', {
    method: 'POST', body: JSON.stringify({ expected_version: 9 }),
  });
});

it('reads removal impact without changing status or fetching upstream models', async () => {
  await fetchCredentialRemovalImpact({ id: 5 } as UnifiedCredential);
  expect(request).toHaveBeenCalledExactlyOnceWith('/admin/unified-gateway/credentials/5/removal-impact');
});
