import {
  fetchCatalogSources,
  prepareCredentialCatalogSource,
  refreshActiveCatalog,
  type CredentialCatalogSourceFields,
  type UnifiedCatalogDiscoveryItem,
  type UnifiedCatalogSource,
} from '../../services/unifiedCatalogSourceApi';
import type { UnifiedCatalogProduct, UnifiedCredential, UnifiedRelationLink } from '../../services/unifiedGatewayApi';

export interface ModelDiscoveryPreset {
  vendorModel: string;
  baseURL: string;
  poolId: number;
  snapshotId: number;
  priceReference?: string;
}

export const credentialCatalogBaseURLs = (
  credential: UnifiedCredential, products: UnifiedCatalogProduct[], relations: UnifiedRelationLink[],
): string[] => {
  const keyRoutes = relations.filter(item => item.channel_id === credential.channel_id && item.credential_id === credential.id);
  const channelProducts = products.filter(item => item.channel_id === credential.channel_id);
  const poolProducts = channelProducts.filter(item => item.credential_pool_id === credential.credential_pool_id);
  const candidates = keyRoutes.length ? keyRoutes : poolProducts.length ? poolProducts : channelProducts;
  const origins = candidates.flatMap(item => {
    try {
      const url = new URL(item.base_url);
      if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) return [];
      // The discovery protocol owns the request path. Only reuse an origin or
      // the standard /v1 prefix; custom gateway prefixes need their own protocol.
      if (!['/', '/v1', '/v1/'].includes(url.pathname)) return [];
      return [url.origin];
    } catch { return []; }
  });
  return [...new Set(origins)].sort();
};

export const newCatalogSourceFields = (
  baseURLs: string[], protocolDefinitionId: number, source?: UnifiedCatalogSource,
): CredentialCatalogSourceFields => source ? {
  base_url: source.base_url, external_group: source.external_group,
  request_timeout_ms: source.request_timeout_ms, protocol_definition_id: source.protocol_definition_id,
} : {
  base_url: baseURLs.length === 1 ? baseURLs[0] : '', external_group: 'default',
  request_timeout_ms: 15000, protocol_definition_id: protocolDefinitionId,
};

export const validateCatalogSourceForm = (fields: CredentialCatalogSourceFields): string => {
  try {
    const url = new URL(fields.base_url);
    if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password ||
      url.pathname !== '/' || url.search || url.hash) throw new Error();
  } catch { return '地址只填写 HTTP/HTTPS 协议与域名，不包含路径、查询参数或账号'; }
  if (!fields.external_group || Array.from(fields.external_group).length > 128 || /[\0\r\n\t]/.test(fields.external_group)) return '上游分组标识无效';
  if (!Number.isInteger(fields.request_timeout_ms) || fields.request_timeout_ms < 1000 || fields.request_timeout_ms > 120000) return '超时需为 1000–120000 毫秒';
  if (!Number.isSafeInteger(fields.protocol_definition_id) || fields.protocol_definition_id <= 0) return '请选择数据库中的模型列表协议';
  return '';
};

export const loadCredentialCatalogSources = async (credentialId: number, signal?: AbortSignal) => {
  const sources: UnifiedCatalogSource[] = [];
  for (let page = 1; ; page += 1) {
    const result = await fetchCatalogSources(page, 100, '', signal, credentialId);
    if (signal?.aborted) return sources;
    sources.push(...result.items);
    if (sources.length >= result.total || result.items.length === 0) return sources;
  }
};

// Only an explicit click reaches this function. Reading a snapshot never does.
export const pullCredentialModelList = async (
  credentialId: number, releaseId: number, sourceId: number | null, fields: CredentialCatalogSourceFields,
) => {
  const source = sourceId || (await prepareCredentialCatalogSource(credentialId, fields)).id;
  const run = await refreshActiveCatalog(releaseId, source);
  return { sourceId: source, runId: run.id };
};

export const discoveryModelPreset = (
  item: UnifiedCatalogDiscoveryItem, source: UnifiedCatalogSource, credential: UnifiedCredential, snapshotId: number,
): ModelDiscoveryPreset => {
  if (!item.selected_group_enabled || source.credential_id !== credential.id || source.channel_id !== credential.channel_id ||
    source.status !== 'active' || credential.status !== 'active' || !credential.credential_pool_id || !snapshotId) {
    throw new Error('此快照不属于当前可用 Key 或当前上游分组');
  }
  // Raw descriptions/prices are evidence, not automatically published metadata
  // or validated prices. The onboarding form still requires those fields.
  return { vendorModel: item.model_code, baseURL: source.base_url, poolId: credential.credential_pool_id, snapshotId, priceReference: catalogPriceFacts(item) };
};

export const catalogRunStateLabel = (state: string) => ({
  scheduled: '排队中', running: '拉取中', completed: '已完成', failed: '失败', manual_review: '需处理',
}[state] || state);

export const catalogPriceFacts = (item: UnifiedCatalogDiscoveryItem) => [
  item.model_price != null ? `单价字段 ${item.model_price}` : '',
  item.model_ratio != null ? `模型倍率 ${item.model_ratio}` : '',
  item.completion_ratio != null ? `补全倍率 ${item.completion_ratio}` : '',
].filter(Boolean).join(' / ') || '未提供';
