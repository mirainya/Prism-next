import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  fetchCatalogSources, prepareCredentialCatalogSource, refreshActiveCatalog,
  type UnifiedCatalogDiscoveryItem, type UnifiedCatalogSource,
} from '../../services/unifiedCatalogSourceApi';
import type { UnifiedCatalogProduct, UnifiedCredential, UnifiedRelationLink } from '../../services/unifiedGatewayApi';
import { catalogPriceFacts, credentialCatalogBaseURLs, discoveryModelPreset, loadCredentialCatalogSources, newCatalogSourceFields, pullCredentialModelList, validateCatalogSourceForm } from './catalogSnapshotForm';

vi.mock('../../services/unifiedCatalogSourceApi', () => ({
  fetchCatalogSources: vi.fn(), prepareCredentialCatalogSource: vi.fn(), refreshActiveCatalog: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());
const fields = { base_url: 'https://provider.example', external_group: 'default', request_timeout_ms: 15000, protocol_definition_id: 70 };

describe('model-list connection defaults', () => {
  const credential = { id: 40, channel_id: 7, credential_pool_id: 24 } as UnifiedCredential;
  const product = (pool: number, url: string, channel = 7) => ({ channel_id: channel, credential_pool_id: pool, base_url: url }) as UnifiedCatalogProduct;
  const relation = (key: number, url: string, channel = 7) => ({ channel_id: channel, credential_id: key, base_url: url }) as UnifiedRelationLink;

  it('uses this Keys own route rather than another Keys address', () => {
    const urls = credentialCatalogBaseURLs(credential,
      [product(24, 'https://other-key.example'), product(25, 'https://another-pool.example')],
      [relation(40, 'https://selected-key.example/v1/'), relation(41, 'https://other-key.example')]);
    expect(urls).toEqual(['https://selected-key.example']);
    expect(newCatalogSourceFields(urls, 70).base_url).toBe('https://selected-key.example');
  });

  it('uses the selected pool before considering other pools in this channel', () => {
    expect(credentialCatalogBaseURLs(credential,
      [product(24, 'https://pool.example'), product(25, 'https://another-pool.example')], [])).toEqual(['https://pool.example']);
  });

  it('a new isolated pool can reuse a unique channel origin without copying another Keys group', () => {
    const urls = credentialCatalogBaseURLs({ ...credential, credential_pool_id: 26 },
      [product(24, 'https://channel.example/'), product(25, 'https://channel.example/v1'), product(26, 'https://unrelated.example', 8)], []);
    expect(newCatalogSourceFields(urls, 70)).toEqual({ ...fields, base_url: 'https://channel.example' });
  });

  it('keeps conflicting addresses as choices and does not choose the first', () => {
    const urls = credentialCatalogBaseURLs(credential, [],
      [relation(40, 'https://first.example'), relation(40, 'https://second.example')]);
    expect(urls).toHaveLength(2);
    expect(newCatalogSourceFields(urls, 70).base_url).toBe('');
  });

  it('does not turn custom prefixes or URLs containing credentials into standard endpoints', () => {
    expect(credentialCatalogBaseURLs(credential, [product(24, 'https://fallback.example')],
      [relation(40, 'https://provider.example/custom-proxy'), relation(40, 'https://user:secret@provider.example')])).toEqual([]);
  });

  it('copies all connection parameters from an existing source for this Key', () => {
    const source = { base_url: 'https://configured.example', external_group: 'wholesale', request_timeout_ms: 22000, protocol_definition_id: 82 } as UnifiedCatalogSource;
    expect(newCatalogSourceFields(['https://channel.example'], 70, source)).toEqual({
      base_url: source.base_url, external_group: source.external_group,
      request_timeout_ms: source.request_timeout_ms, protocol_definition_id: source.protocol_definition_id,
    });
  });
});

describe('credential model discovery', () => {
  it('reading all source pages is scoped to one Key and never pulls upstream', async () => {
    vi.mocked(fetchCatalogSources).mockResolvedValueOnce({ items: [{ id: 12 }], total: 2 } as never)
      .mockResolvedValueOnce({ items: [{ id: 13 }], total: 2 } as never);
    const signal = new AbortController().signal;
    expect(await loadCredentialCatalogSources(40, signal)).toEqual([{ id: 12 }, { id: 13 }]);
    expect(fetchCatalogSources).toHaveBeenNthCalledWith(1, 1, 100, '', signal, 40);
    expect(fetchCatalogSources).toHaveBeenNthCalledWith(2, 2, 100, '', signal, 40);
    expect(prepareCredentialCatalogSource).not.toHaveBeenCalled();
    expect(refreshActiveCatalog).not.toHaveBeenCalled();
  });

  it('stops reading a closed dialog without authorizing or refreshing', async () => {
    const controller = new AbortController();
    vi.mocked(fetchCatalogSources).mockImplementationOnce(async () => {
      controller.abort(); return { items: [{ id: 12 }], total: 100 } as never;
    });
    expect(await loadCredentialCatalogSources(40, controller.signal)).toEqual([]);
    expect(fetchCatalogSources).toHaveBeenCalledTimes(1);
    expect(refreshActiveCatalog).not.toHaveBeenCalled();
  });

  it('an existing source refresh does not grant or create configuration', async () => {
    vi.mocked(refreshActiveCatalog).mockResolvedValue({ id: 51 });
    expect(await pullCredentialModelList(40, 7, 12, fields)).toEqual({ sourceId: 12, runId: 51 });
    expect(prepareCredentialCatalogSource).not.toHaveBeenCalled();
    expect(refreshActiveCatalog).toHaveBeenCalledExactlyOnceWith(7, 12);
  });

  it('a first explicit pull saves the selected Keys configuration before refreshing', async () => {
    vi.mocked(prepareCredentialCatalogSource).mockResolvedValue({ id: 12 });
    vi.mocked(refreshActiveCatalog).mockResolvedValue({ id: 51 });
    await pullCredentialModelList(40, 7, null, fields);
    expect(prepareCredentialCatalogSource).toHaveBeenCalledExactlyOnceWith(40, fields);
    expect(refreshActiveCatalog).toHaveBeenCalledExactlyOnceWith(7, 12);
    expect(vi.mocked(prepareCredentialCatalogSource).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(refreshActiveCatalog).mock.invocationCallOrder[0]);
  });

  it('configuration failure never falls back to another Key or source', async () => {
    vi.mocked(prepareCredentialCatalogSource).mockRejectedValue(new Error('configuration failure'));
    await expect(pullCredentialModelList(40, 7, null, fields)).rejects.toThrow('configuration failure');
    expect(refreshActiveCatalog).not.toHaveBeenCalled();
  });

  it('refresh failure is reported and is not automatically retried', async () => {
    vi.mocked(refreshActiveCatalog).mockRejectedValue(new Error('refresh failure'));
    await expect(pullCredentialModelList(40, 7, 12, fields)).rejects.toThrow('refresh failure');
    expect(refreshActiveCatalog).toHaveBeenCalledTimes(1);
  });

  it('requires an origin, valid protocol and bounded timeout', () => {
    expect(validateCatalogSourceForm(fields)).toBe('');
    for (const base_url of ['file:///key', 'https://user:secret@host', 'https://host/v1', 'https://host?key=secret', 'https://host#x']) {
      expect(validateCatalogSourceForm({ ...fields, base_url })).not.toBe('');
    }
    for (const request_timeout_ms of [0, 999, 120001, NaN, 1000.5]) {
      expect(validateCatalogSourceForm({ ...fields, request_timeout_ms })).not.toBe('');
    }
    expect(validateCatalogSourceForm({ ...fields, protocol_definition_id: 0 })).not.toBe('');
    expect(validateCatalogSourceForm({ ...fields, external_group: 'bad\ngroup' })).not.toBe('');
  });
});

describe('snapshot onboarding preset', () => {
  const item = { id: 1, model_code: 'model-a', selected_group_enabled: true, description: 'private upstream description', model_price: '9.9' } as UnifiedCatalogDiscoveryItem;
  const source = { id: 12, credential_id: 40, channel_id: 7, base_url: 'https://provider.example', status: 'active' } as UnifiedCatalogSource;
  const credential = { id: 40, channel_id: 7, credential_pool_id: 20, status: 'active' } as UnifiedCredential;

  it('preserves price facts and never treats unknown prices as zero', () => {
    expect(catalogPriceFacts({ ...item, model_price: null, model_ratio: null, completion_ratio: null })).toBe('未提供');
    expect(catalogPriceFacts({ ...item, model_price: null, model_ratio: '15', completion_ratio: '3' })).toBe('模型倍率 15 / 补全倍率 3');
    expect(catalogPriceFacts({ ...item, model_price: '0', model_ratio: null, completion_ratio: null })).toBe('单价字段 0');
  });

  it('keeps the chosen Keys pool and does not automatically publish descriptions or prices', () => {
    expect(discoveryModelPreset(item, source, credential, 61)).toEqual({ vendorModel: 'model-a', baseURL: 'https://provider.example', poolId: 20, snapshotId: 61, priceReference: catalogPriceFacts(item) });
  });

  it('never substitutes another Key, channel or unsupported group', () => {
    expect(() => discoveryModelPreset(item, { ...source, credential_id: 41 }, credential, 61)).toThrow();
    expect(() => discoveryModelPreset(item, { ...source, channel_id: 8 }, credential, 61)).toThrow();
    expect(() => discoveryModelPreset({ ...item, selected_group_enabled: false }, source, credential, 61)).toThrow();
    expect(() => discoveryModelPreset(item, { ...source, status: 'disabled' }, credential, 61)).toThrow();
    expect(() => discoveryModelPreset(item, source, { ...credential, status: 'disabled' }, 61)).toThrow();
  });
});
