import { afterEach, expect, it, vi } from 'vitest';

import { request } from './request';
import { bindTokenFileStorage, deleteToken, fetchTokenFileStorageEndpoints, fetchTokens, unbindTokenFileStorage } from './tokenApi';

vi.mock('./request', () => ({ request: vi.fn() }));

afterEach(() => vi.clearAllMocks());

it('maps the per-token file storage status without exposing a full key', async () => {
  vi.mocked(request).mockResolvedValueOnce([{
    id: 7,
    name: '生产环境',
    key: 'prism_****1234',
    balance: '12.5',
    total_used: '1.25',
    status: 1,
    xfs_storage: { configured: true, key_hint: '****dfc0', base_url: 'https://xfs.example.com' },
  }]);

  await expect(fetchTokens()).resolves.toEqual([expect.objectContaining({
    id: '7',
    xfsStorage: { configured: true, keyHint: '****dfc0', baseURL: 'https://xfs.example.com' },
  })]);
});

it('binds and unbinds storage through the token-scoped endpoints', async () => {
  vi.mocked(request)
    .mockResolvedValueOnce(['https://xfs.example.com'])
    .mockResolvedValueOnce({ configured: true, key_hint: '****dfc0', base_url: 'https://xfs.example.com' })
    .mockResolvedValueOnce({ configured: false, key_hint: '', base_url: 'https://xfs.example.com' });

  await expect(fetchTokenFileStorageEndpoints()).resolves.toEqual(['https://xfs.example.com']);
  await expect(bindTokenFileStorage('7', 'https://xfs.example.com', 'xfs_test')).resolves.toMatchObject({ configured: true, keyHint: '****dfc0', baseURL: 'https://xfs.example.com' });
  expect(request).toHaveBeenNthCalledWith(2, '/tokens/7/file-storage', {
    method: 'PUT',
    body: JSON.stringify({ base_url: 'https://xfs.example.com', api_key: 'xfs_test' }),
  });

  await expect(unbindTokenFileStorage('7')).resolves.toMatchObject({ configured: false });
  expect(request).toHaveBeenNthCalledWith(3, '/tokens/7/file-storage', { method: 'DELETE' });
});

it('deletes the selected token through its scoped endpoint', async () => {
  vi.mocked(request).mockResolvedValueOnce({ deleted: true });

  await expect(deleteToken('7')).resolves.toBeUndefined();
  expect(request).toHaveBeenCalledWith('/tokens/7', { method: 'DELETE' });
});
