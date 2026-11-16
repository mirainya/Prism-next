import { beforeEach, expect, it, vi } from 'vitest';
import { request } from './request';
import { checkSystemUpdate, downloadSystemUpdate, fetchSystemUpdateStatus, fetchSystemVersion, fetchUpdateSources, saveUpdateSources, prepareSystemUpdate, installSystemUpdate } from './systemUpdateApi';

vi.mock('./request', () => ({ request: vi.fn() }));

beforeEach(() => vi.mocked(request).mockReset());

it('requires a reviewed version and migration digest when requesting installation', async () => {
  await prepareSystemUpdate();
  expect(request).toHaveBeenCalledWith('/admin/system/update/prepare', { method: 'POST', body: '{}', signal: undefined });
  await installSystemUpdate('2.0.14', 'reviewed-digest');
  expect(request).toHaveBeenLastCalledWith('/admin/system/update/install', {
    method: 'POST', body: JSON.stringify({ version: '2.0.14', plan_digest: 'reviewed-digest', confirmed: true }), signal: undefined,
  });
});

it('keeps the source list revision and immutable GitHub source for safe configuration edits', async () => {
  const github = { id: 0, name: 'github', base_url: 'https://github.com', enabled: true, priority: 100000, config_version: 1, built_in: true };
  vi.mocked(request).mockResolvedValueOnce({ sources: [github], revision: 4 });
  const config = await fetchUpdateSources();
  expect(config).toEqual({ sources: [github], revision: 4 });
  expect(request).toHaveBeenCalledWith('/admin/system/update-sources', { signal: undefined });

  await saveUpdateSources(config.sources, config.revision);
  expect(request).toHaveBeenLastCalledWith('/admin/system/update-sources', {
    method: 'PUT', body: JSON.stringify({ sources: [github], expected_revision: 4 }), signal: undefined,
  });
});

it('submits a background download without accepting a client filesystem path and reads its state', async () => {
  vi.mocked(request).mockResolvedValueOnce({ state: 'downloading' }).mockResolvedValueOnce({ state: 'staged', version: '2.0.14' });
  expect(await downloadSystemUpdate()).toEqual({ state: 'downloading' });
  expect(request).toHaveBeenCalledWith('/admin/system/update/download', { method: 'POST', body: '{}', signal: undefined });
  expect(await fetchSystemUpdateStatus()).toEqual({ state: 'staged', version: '2.0.14' });
  expect(request).toHaveBeenLastCalledWith('/admin/system/update/status', { signal: undefined });
});

it('reads the running version and checks releases through administrator endpoints', async () => {
  await fetchSystemVersion();
  await checkSystemUpdate();
  expect(request).toHaveBeenNthCalledWith(1, '/admin/system/version', { signal: undefined });
  expect(request).toHaveBeenNthCalledWith(2, '/admin/system/update/check', { signal: undefined });
});
