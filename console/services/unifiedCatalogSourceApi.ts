import { request } from './request';
import type { UnifiedGatewayPage } from './unifiedGatewayApi';
import type { UnifiedAvailabilitySource } from './unifiedGatewayApi';

export type CatalogSourceContract =
  | 'declarative_catalog_v1';

export type AvailabilitySourceContract =
  | 'declarative_availability_v1';

export interface UnifiedProtocolDefinitionOption {
  id: number;
  protocol_code: string;
  protocol_version: number;
  protocol_kind: 'catalog_discovery' | 'availability';
  display_name: string;
}

export interface UnifiedCatalogSource {
  id: number;
  channel_id: number;
  channel_code: string;
  channel_name: string;
  source_code: string;
  contract_code: CatalogSourceContract;
  protocol_definition_id: number;
  base_url: string;
  external_group: string;
  request_timeout_ms: number;
  credential_id: number;
  credential_code: string;
  status: 'active' | 'draining' | 'disabled';
  state_version: number;
  nonterminal_run_count: number;
  last_run_id: number | null;
  last_run_state: string | null;
  last_run_reason: string | null;
  created_at: string;
  updated_at: string;
}

export interface UnifiedCatalogSourceOptions {
  channels: { id: number; code: string; name: string }[];
  credentials: { id: number; channel_id: number; code: string }[];
  contracts: { code: CatalogSourceContract; name: string }[];
  protocol_definitions: UnifiedProtocolDefinitionOption[];
}

export interface UnifiedAvailabilitySourceOptions {
  channels: { id: number; code: string; name: string }[];
  credentials: { id: number; channel_id: number; code: string }[];
  contracts: { code: AvailabilitySourceContract; name: string }[];
  protocol_definitions: UnifiedProtocolDefinitionOption[];
}

export interface UnifiedCatalogDiscovery {
  id: number;
  release_source_id: number;
  source_id: number;
  source_code: string;
  contract_code: CatalogSourceContract;
  external_group: string;
  state: string;
  state_version: number;
  attempt_count: number;
  last_reason_code: string;
  snapshot_id: number | null;
  item_count: number | null;
  observed_at: string | null;
  response_hmac: string | null;
  review_state: 'submitted' | 'accepted' | 'rejected' | null;
  review_version: number | null;
  created_at: string;
  updated_at: string;
}

export interface UnifiedCatalogDiscoveryItem {
  id: number;
  ordinal: number;
  model_code: string;
  description: string;
  tags: string;
  vendor_id: number | null;
  provider_quota_type: number | null;
  model_price: string | null;
  model_ratio: string | null;
  completion_ratio: string | null;
  owner_by: string;
  pricing_version: string;
  selected_group_enabled: boolean;
  groups: string[];
  endpoint_types: string[];
}

export interface UnifiedCatalogPriceCandidate {
  id: number;
  snapshot_id: number;
  model_code: string;
  model_price: string;
  provider_quota_type: number | null;
  observed_at: string;
  source_code: string;
  external_group: string;
  state: 'pending' | 'confirmed' | 'dismissed';
  unit_code: string | null;
  currency_code: string | null;
  currency_version: number | null;
  rate_evidence_id: number | null;
  reviewed_at: string | null;
}

const pageQuery = (page: number, pageSize: number) =>
  `page=${page}&page_size=${pageSize}`;

export const fetchCatalogSourceOptions = (signal?: AbortSignal) =>
  request<UnifiedCatalogSourceOptions>(
    '/admin/unified-gateway/catalog-sources/options',
    { signal },
  );

export const fetchCatalogSources = (
  page: number,
  pageSize: number,
  status = '',
  signal?: AbortSignal,
  credentialId?: number,
) =>
  request<UnifiedGatewayPage<UnifiedCatalogSource>>(
    `/admin/unified-gateway/catalog-sources?${pageQuery(page, pageSize)}&status=${encodeURIComponent(status)}${credentialId ? `&credential_id=${credentialId}` : ''}`,
    { signal },
  );

export interface CredentialCatalogSourceFields {
  base_url: string;
  external_group: string;
  request_timeout_ms: number;
  protocol_definition_id: number;
}

export const prepareCredentialCatalogSource = (credentialId: number, data: CredentialCatalogSourceFields) =>
  request<{ id: number }>(`/admin/unified-gateway/credentials/${credentialId}/catalog-source`, {
    method: 'POST', body: JSON.stringify(data),
  });

export const createCatalogSource = (data: {
  channel_id: number;
  credential_id: number;
  source_code: string;
  contract_code: CatalogSourceContract;
  base_url: string;
  external_group: string;
  request_timeout_ms: number;
  protocol_definition_id?: number;
}) =>
  request<{ id: number }>('/admin/unified-gateway/catalog-sources', {
    method: 'POST',
    body: JSON.stringify(data),
  });

export const transitionCatalogSource = (source: UnifiedCatalogSource) =>
  request<{ id: number }>(
    `/admin/unified-gateway/catalog-sources/${source.id}/status`,
    {
      method: 'POST',
      body: JSON.stringify({
        status: source.status === 'active' ? 'draining' : 'disabled',
        expected_version: source.state_version,
        reason_code: 'admin_transition',
      }),
    },
  );

export const fetchAvailabilitySourceOptions = (signal?: AbortSignal) =>
  request<UnifiedAvailabilitySourceOptions>(
    '/admin/unified-gateway/availability-sources/options',
    { signal },
  );

export const fetchAvailabilitySources = (
  page: number,
  pageSize: number,
  status = '',
  signal?: AbortSignal,
) =>
  request<UnifiedGatewayPage<UnifiedAvailabilitySource>>(
    `/admin/unified-gateway/availability-sources?page=${page}&page_size=${pageSize}&status=${encodeURIComponent(status)}`,
    { signal },
  );

export const createAvailabilitySource = (data: {
  channel_id: number;
  credential_id: number;
  source_code: string;
  contract_code: AvailabilitySourceContract;
  base_url: string;
  request_timeout_ms: number;
  protocol_definition_id?: number;
}) =>
  request<{ id: number }>('/admin/unified-gateway/availability-sources', {
    method: 'POST',
    body: JSON.stringify(data),
  });

export const transitionAvailabilitySource = (source: UnifiedAvailabilitySource) =>
  request<{ id: number }>(
    `/admin/unified-gateway/availability-sources/${source.id}/status`,
    {
      method: 'POST',
      body: JSON.stringify({
        status: source.status === 'active' ? 'disabled' : 'active',
        expected_version: source.config_version,
      }),
    },
  );

export const updateAvailabilitySource = (source: UnifiedAvailabilitySource, data: {
  contract_code?: AvailabilitySourceContract;
  credential_id: number;
  base_url: string;
  request_timeout_ms: number;
  protocol_definition_id?: number;
}) =>
  request<{ id: number }>(`/admin/unified-gateway/availability-sources/${source.id}`, {
    method: 'PUT',
    body: JSON.stringify({ ...data, expected_version: source.config_version }),
  });

export const scheduleCatalogDiscovery = (
  releaseId: number,
  sourceId: number,
) =>
  request<{ id: number }>(
    `/admin/unified-gateway/catalog/${releaseId}/catalog-sources/${sourceId}/discover`,
    { method: 'POST', body: '{}' },
  );

export const fetchCatalogDiscoveries = (
  releaseId: number,
  page: number,
  pageSize: number,
  signal?: AbortSignal,
  filters: { sourceId?: number; credentialId?: number } = {},
) =>
  request<UnifiedGatewayPage<UnifiedCatalogDiscovery>>(
    `/admin/unified-gateway/catalog/${releaseId}/discoveries?${pageQuery(page, pageSize)}${filters.sourceId ? `&source_id=${filters.sourceId}` : ''}${filters.credentialId ? `&credential_id=${filters.credentialId}` : ''}`,
    { signal },
  );

// Unlike the legacy draft discovery, a refresh is read-only and repeatable.
export const refreshActiveCatalog = (releaseId: number, sourceId: number) =>
  request<{ id: number }>(
    `/admin/unified-gateway/catalog/${releaseId}/catalog-sources/${sourceId}/refresh`,
    { method: 'POST', body: '{}' },
  );

export const fetchCatalogDiscoveryItems = (
  snapshotId: number,
  page: number,
  pageSize: number,
  signal?: AbortSignal,
  query = '',
) =>
  request<UnifiedGatewayPage<UnifiedCatalogDiscoveryItem>>(
    `/admin/unified-gateway/catalog-discoveries/${snapshotId}/items?${pageQuery(page, pageSize)}${query ? `&q=${encodeURIComponent(query)}` : ''}`,
    { signal },
  );

export const reviewCatalogDiscovery = (
  discovery: UnifiedCatalogDiscovery,
  decision: 'accepted' | 'rejected',
) =>
  request<{ id: number }>(
    `/admin/unified-gateway/catalog-discoveries/${discovery.snapshot_id}/review`,
    {
      method: 'POST',
      body: JSON.stringify({
        decision,
        reason_code: decision === 'accepted' ? 'admin_accepted' : 'admin_rejected',
        expected_version: discovery.review_version,
      }),
    },
  );

export const fetchCatalogPriceCandidates = (
  releaseId: number,
  page: number,
  pageSize: number,
  state = '',
  signal?: AbortSignal,
) =>
  request<UnifiedGatewayPage<UnifiedCatalogPriceCandidate>>(
    `/admin/unified-gateway/catalog/${releaseId}/price-candidates?${pageQuery(page, pageSize)}&state=${encodeURIComponent(state)}`,
    { signal },
  );

export const reviewCatalogPriceCandidate = (
  candidateId: number,
  data:
    | {
        decision: 'confirmed';
        unit_code: string;
        currency_code: string;
        currency_version: number;
        reason_code: string;
      }
    | {
        decision: 'dismissed';
        unit_code: '';
        currency_code: '';
        currency_version: 0;
        reason_code: string;
      },
) =>
  request<{ id: number }>(
    `/admin/unified-gateway/catalog-price-candidates/${candidateId}/review`,
    { method: 'POST', body: JSON.stringify(data) },
  );

export interface UnifiedProtocolDefinition extends UnifiedProtocolDefinitionOption {
  status: 'active' | 'disabled';
  config_version: number;
  definition_digest: string;
  definition?: unknown;
}
export const fetchProtocolDefinitions = (kind: string, signal?: AbortSignal) =>
  request<UnifiedGatewayPage<UnifiedProtocolDefinition>>(`/admin/unified-gateway/protocol-definitions?page=1&page_size=100&kind=${encodeURIComponent(kind)}`, { signal });
export const readProtocolDefinition = (id: number) =>
  request<UnifiedProtocolDefinition>(`/admin/unified-gateway/protocol-definitions/${id}`);
export const createProtocolDefinition = (data: { protocol_code: string; protocol_version: number; protocol_kind: 'availability' | 'catalog_discovery'; display_name: string; definition: unknown }) =>
  request<{ id: number }>('/admin/unified-gateway/protocol-definitions', { method: 'POST', body: JSON.stringify(data) });
export const transitionProtocolDefinition = (item: UnifiedProtocolDefinition) =>
  request(`/admin/unified-gateway/protocol-definitions/${item.id}/status`, { method: 'POST', body: JSON.stringify({ status: item.status === 'active' ? 'disabled' : 'active', expected_version: item.config_version }) });
export const updateCatalogSourceProtocol = (source: UnifiedCatalogSource, definitionID: number) =>
  request(`/admin/unified-gateway/catalog-sources/${source.id}/protocol`, { method: 'PUT', body: JSON.stringify({ contract_code: 'declarative_catalog_v1', protocol_definition_id: definitionID, expected_version: source.state_version }) });
