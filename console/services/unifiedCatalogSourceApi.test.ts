import { afterEach, describe, expect, it, vi } from 'vitest';
import { request } from './request';
import {
  fetchCatalogPriceCandidates,
  fetchCatalogDiscoveries,
  fetchCatalogDiscoveryItems,
  refreshActiveCatalog,
  fetchCatalogSources,
  prepareCredentialCatalogSource,
  createAvailabilitySource,
  transitionAvailabilitySource,
  updateAvailabilitySource,
  reviewCatalogDiscovery,
  scheduleCatalogDiscovery,
  transitionCatalogSource,
  type UnifiedCatalogDiscovery,
  type UnifiedCatalogSource,
} from './unifiedCatalogSourceApi';
import type { UnifiedAvailabilitySource } from './unifiedGatewayApi';

vi.mock('./request', () => ({ request: vi.fn().mockResolvedValue({}) }));

afterEach(() => vi.clearAllMocks());

describe('unified catalog source API', () => {
  it('filters source configuration by the actual Key, not its pool', async () => {
    await fetchCatalogSources(1, 100, '', undefined, 40);
    expect(request).toHaveBeenCalledWith('/admin/unified-gateway/catalog-sources?page=1&page_size=100&status=&credential_id=40', { signal: undefined });
  });

  it('saves explicit first-time discovery configuration only for the selected Key', async () => {
    const fields = { base_url: 'https://provider.example', external_group: 'default', request_timeout_ms: 15000, protocol_definition_id: 70 };
    await prepareCredentialCatalogSource(40, fields);
    expect(request).toHaveBeenCalledWith('/admin/unified-gateway/credentials/40/catalog-source', { method: 'POST', body: JSON.stringify(fields) });
  });

  it('searches the saved snapshot through Prism rather than the upstream', async () => {
    await fetchCatalogDiscoveryItems(61, 2, 50, undefined, 'model-a / x');
    expect(request).toHaveBeenCalledWith('/admin/unified-gateway/catalog-discoveries/61/items?page=2&page_size=50&q=model-a%20%2F%20x', { signal: undefined });
    expect(request).toHaveBeenCalledTimes(1);
  });

  it('refreshes the active catalog without a draft or publication', async () => {
    await refreshActiveCatalog(7, 12);
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog/7/catalog-sources/12/refresh',
      { method: 'POST', body: '{}' },
    );
  });

  it('reads stored discovery history scoped to the chosen source and Key', async () => {
    const signal = new AbortController().signal;
    await fetchCatalogDiscoveries(7, 1, 20, signal, { sourceId: 12, credentialId: 40 });
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog/7/discoveries?page=1&page_size=20&source_id=12&credential_id=40',
      { signal },
    );
  });

  it('opening a saved snapshot only reads Prism and does not refresh the source', async () => {
    await fetchCatalogDiscoveryItems(51, 1, 100);
    expect(request).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog-discoveries/51/items?page=1&page_size=100',
      { signal: undefined },
    );
  });

  it('creates a monitor without using the catalog discovery endpoint', async () => {
    await createAvailabilitySource({
      channel_id: 4,
      credential_id: 8,
      source_code: 'health-main',
      contract_code: 'declarative_availability_v1',
      base_url: 'https://provider.example',
      request_timeout_ms: 15000,
          protocol_definition_id: 22,
    });
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/availability-sources',
      {
        method: 'POST',
        body: JSON.stringify({
          channel_id: 4,
          credential_id: 8,
          source_code: 'health-main',
          contract_code: 'declarative_availability_v1',
          base_url: 'https://provider.example',
          request_timeout_ms: 15000,
          protocol_definition_id: 22,
        }),
      },
    );
  });

  it('toggles a monitor using its config version', async () => {
    await transitionAvailabilitySource({
      id: 14,
      status: 'active',
      config_version: 3,
    } as UnifiedAvailabilitySource);
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/availability-sources/14/status',
      {
        method: 'POST',
        body: JSON.stringify({ status: 'disabled', expected_version: 3 }),
      },
    );
  });

  it('edits monitor connection settings with an optimistic config version', async () => {
    await updateAvailabilitySource({ id: 14, config_version: 3 } as UnifiedAvailabilitySource, {
      credential_id: 8,
      base_url: 'https://new.example.com',
      request_timeout_ms: 12000,
    });
    expect(request).toHaveBeenCalledWith('/admin/unified-gateway/availability-sources/14', {
      method: 'PUT',
      body: JSON.stringify({
        credential_id: 8,
        base_url: 'https://new.example.com',
        request_timeout_ms: 12000,
        expected_version: 3,
      }),
    });
  });

  it('schedules a source inside the selected immutable draft', async () => {
    await scheduleCatalogDiscovery(7, 9);
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog/7/catalog-sources/9/discover',
      { method: 'POST', body: '{}' },
    );
  });

  it('uses the observed source version while draining', async () => {
    await transitionCatalogSource({
      id: 9,
      status: 'active',
      state_version: 4,
    } as UnifiedCatalogSource);
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog-sources/9/status',
      {
        method: 'POST',
        body: JSON.stringify({
          status: 'draining',
          expected_version: 4,
          reason_code: 'admin_transition',
        }),
      },
    );
  });

  it('reviews the exact snapshot version', async () => {
    await reviewCatalogDiscovery(
      { snapshot_id: 12, review_version: 3 } as UnifiedCatalogDiscovery,
      'accepted',
    );
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog-discoveries/12/review',
      {
        method: 'POST',
        body: JSON.stringify({
          decision: 'accepted',
          reason_code: 'admin_accepted',
          expected_version: 3,
        }),
      },
    );
  });

  it('encodes price candidate filters', async () => {
    const controller = new AbortController();
    await fetchCatalogPriceCandidates(5, 2, 50, 'pending & new', controller.signal);
    expect(request).toHaveBeenCalledWith(
      '/admin/unified-gateway/catalog/5/price-candidates?page=2&page_size=50&state=pending%20%26%20new',
      { signal: controller.signal },
    );
  });
});
