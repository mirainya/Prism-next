import type {
  UnifiedAdapterManifest,
  UnifiedCatalogModelOnboardInput,
  UnifiedCatalogModelOnboardRate,
  UnifiedCatalogModelType,
} from '../../services/unifiedGatewayApi';

const identityPattern = /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$/;
const publicModelNamePattern = /^[A-Za-z0-9][A-Za-z0-9._:/%\-图]*$/u;
const pricePattern = /^(?:0|[1-9]\d{0,17})(?:\.\d{1,18})?$/;

export const isValidPublicModelName = (value: string) =>
  publicModelNamePattern.test(value) && value === value.trim() && new TextEncoder().encode(value).length <= 128;

const downstreamOperations: Record<string, { operationCode: string; maxResults: number }> = {
  '/v1/chat/completions': { operationCode: 'chat.completions', maxResults: 1 },
  '/v1/responses': { operationCode: 'responses.create', maxResults: 1 },
  '/v1/messages': { operationCode: 'messages.create', maxResults: 1 },
  '/v1/images/generations': { operationCode: 'images.generate', maxResults: 10 },
  '/v1/images/edits': { operationCode: 'images.edit', maxResults: 10 },
  '/v1/videos/generations': { operationCode: 'videos.generate', maxResults: 1 },
};

export interface OnboardingVariantChoice {
  code: string;
  label: string;
}

export interface OnboardingManifestChoices {
  downstreamPaths: string[];
  variants: OnboardingVariantChoice[];
}

export const modelTypeForAdapter = (adapterCode: string, capability = ''): UnifiedCatalogModelType => {
  if (capability === 'video' || adapterCode === 'generic' || adapterCode === 'seedance') return 'video';
  if (capability === 'image' || adapterCode === 'openai_images') return 'image';
  return 'llm';
};

export const onboardingManifestChoices = (
  adapterCode: string,
  manifest: UnifiedAdapterManifest | null,
): OnboardingManifestChoices => {
  const declaredPaths = manifest?.has_manifest
    ? (manifest.downstream_paths || []).filter((path) => downstreamOperations[path])
    : [];
  const downstreamPaths = declaredPaths.length > 0
    ? declaredPaths
    : adapterCode === 'generic' ? ['/v1/videos/generations'] : [];
  const declaredVariants = manifest?.has_manifest
    ? (manifest.variants || []).filter((variant) => identityPattern.test(variant.code))
    : [];
  const variants = declaredVariants.length > 0
    ? declaredVariants.map((variant) => ({ code: variant.code, label: variant.display || variant.code }))
    : [{ code: 'default', label: '默认' }];
  return { downstreamPaths, variants };
};

export interface NewModelOnboardingValues {
  releaseId: number;
  configVersion: number;
  channelId: number;
  channelCode: string;
  poolId: number;
  poolCode: string;
  adapterCode: string;
  adapterVersion: number;
  adapterCapability: string;
  apiName: string;
  displayName: string;
  vendorModel: string;
  baseURL: string;
  requestPath: string;
  downstreamPath: string;
  variantCode: string;
  capabilityConstraints: Record<string, unknown>;
  billingUnit: '' | 'request' | 'second' | 'token';
  billingMaxQuantity: string;
  sellPrice: string;
  costPrice?: string;
  tokenPricing?: LLMTokenPricingValues;
  llmCapabilities?: LLMCapabilityValues;
  uniqueSuffix: string;
}

export interface LLMTokenPricingValues {
  inputSellPrice: string;
  outputSellPrice: string;
  cacheReadSellPrice?: string;
  cacheWriteSellPrice?: string;
  inputCostPrice?: string;
  outputCostPrice?: string;
  maxInputTokens: string;
  maxOutputTokens: string;
}

export interface LLMCapabilityValues {
  contextWindow: string;
  supportsStreaming: boolean;
  supportsTools: boolean;
  supportsVision: boolean;
  supportsAudio: boolean;
  supportsReasoning: boolean;
}

export const emptyLLMCapabilities = (): LLMCapabilityValues => ({
  contextWindow: '', supportsStreaming: false, supportsTools: false,
  supportsVision: false, supportsAudio: false, supportsReasoning: false,
});

export const emptyLLMTokenPricing = (): LLMTokenPricingValues => ({
  inputSellPrice: '', outputSellPrice: '', cacheReadSellPrice: '', cacheWriteSellPrice: '',
  maxInputTokens: '', maxOutputTokens: '',
});

const isPositivePrice = (value: string) => pricePattern.test(value) && Number(value) > 0;

export interface OnboardingValidationIssue { field: string; message: string }

export const modelOnboardingIssue = (values: NewModelOnboardingValues): OnboardingValidationIssue | null => {
  const issue = (field: string, message: string) => ({ field, message });
  const apiName = values.apiName.trim();
  const displayName = values.displayName.trim();
  const vendorModel = values.vendorModel.trim();
  const requestPath = values.requestPath.trim();
  if (!values.releaseId || !values.configVersion || !values.channelId || !values.poolId || !values.poolCode) return issue('poolID', '请选择有效的 Key 池');
  if (!isValidPublicModelName(apiName)) return issue('apiName', '公开调用名须以字母或数字开头，最多 128 字节，仅支持字母、数字、图或 . _ : / - %');
  if (!displayName || [...displayName].length > 128 || /[\x00\r\n\t]/.test(displayName)) return issue('displayName', '显示名须为 1-128 个字符');
  if (!vendorModel || vendorModel.length > 255 || /[\x00\r\n\t]/.test(vendorModel)) return issue('vendorModel', '请填写有效的上游模型');
  try {
    const endpoint = new URL(values.baseURL.trim());
    if (!['http:', 'https:'].includes(endpoint.protocol) || !endpoint.hostname || endpoint.username || endpoint.password || endpoint.search || endpoint.hash) throw new Error();
  } catch {
    return issue('baseURL', '上游地址必须是无查询参数的 HTTP 或 HTTPS 地址');
  }
  if (requestPath.length > 255 || !requestPath.startsWith('/') || requestPath.startsWith('//') || /[#\x00\r\n\s]/.test(requestPath)) return issue('requestPath', '请填写有效的上游请求路径');
  if (!downstreamOperations[values.downstreamPath]) return issue('downstreamPath', '请选择 Adapter 声明的下游入口');
  if (values.variantCode.length > 64 || !identityPattern.test(values.variantCode)) return issue('variantCode', '请选择有效的模型规格');
  if (!['request', 'second', 'token'].includes(values.billingUnit)) return issue('billingUnit', '请选择计费单位');
  if (values.billingUnit === 'second') {
    if (modelTypeForAdapter(values.adapterCode.trim(), values.adapterCapability) !== 'video') return issue('billingUnit', '按秒计费仅支持视频模型');
    const maxQuantity = values.billingMaxQuantity.trim();
    if (!/^[1-9]\d*$/.test(maxQuantity) || Number(maxQuantity) > 86400) return issue('billingMaxQuantity', '按秒计费上限须为 1-86400 的整数');
  }
  if (values.billingUnit === 'token') {
    if (modelTypeForAdapter(values.adapterCode.trim(), values.adapterCapability) !== 'llm') return issue('billingUnit', 'Token 计费仅支持 LLM 模型');
    const token = values.tokenPricing;
    if (!token) return issue('inputSellPrice', '请填写输入、输出价格与 Token 预留上限');
    for (const [field, label, price, required] of [
      ['inputSellPrice', '输入价格', token.inputSellPrice, true], ['outputSellPrice', '输出价格', token.outputSellPrice, true],
      ['cacheReadSellPrice', '缓存读取价格', token.cacheReadSellPrice, false], ['cacheWriteSellPrice', '缓存写入价格', token.cacheWriteSellPrice, false],
      ['inputCostPrice', '输入成本', token.inputCostPrice, false], ['outputCostPrice', '输出成本', token.outputCostPrice, false],
    ] as const) {
      const value = price?.trim() || '';
      if (!value && !required) continue;
      if (!value) return issue(field, `请填写${label}`);
      if (!pricePattern.test(value)) return issue(field, `${label}请输入非负十进制数，最多 18 位小数`);
    }
    for (const [field, label, value] of [
      ['maxInputTokens', '输入 Token 预留上限', token.maxInputTokens],
      ['maxOutputTokens', '输出 Token 预留上限', token.maxOutputTokens],
    ]) {
      if (!value.trim()) return issue(field, `请填写${label}`);
      if (!/^[1-9]\d{0,17}$/.test(value.trim())) return issue(field, `${label}请输入大于零的整数，最多 18 位`);
    }
    if (!token.inputCostPrice?.trim() && token.outputCostPrice?.trim()) return issue('inputCostPrice', '填写输出成本时请同时提供输入成本');
  } else {
    if (!isPositivePrice(values.sellPrice.trim())) return issue('sellPrice', '价格须为大于 0 的十进制数');
    if (values.costPrice?.trim() && !isPositivePrice(values.costPrice.trim())) return issue('costPrice', '成本须为大于 0 的十进制数');
  }
  if (values.llmCapabilities?.contextWindow.trim()) {
    const contextWindow = values.llmCapabilities.contextWindow.trim();
    if (!/^[1-9]\d{0,15}$/.test(contextWindow) || !Number.isSafeInteger(Number(contextWindow))) return issue('contextWindow', '上下文上限须为精确正整数，最多 9007199254740991');
  }
  if (!values.capabilityConstraints || Array.isArray(values.capabilityConstraints)) return issue('mapping', '参数与能力配置必须是 JSON 对象');
  return null;
};

export const validateNewModelOnboarding = (values: NewModelOnboardingValues): string => modelOnboardingIssue(values)?.message || '';

const identityPart = (value: string) => value.toLowerCase()
  .replace(/[^a-z0-9]+/g, '-')
  .replace(/^-+|-+$/g, '') || 'model';

const generatedCode = (parts: string[], suffix: string) => {
  const tail = `-${identityPart(suffix)}`;
  const stem = parts.map(identityPart).join('-');
  return `${stem.slice(0, Math.max(1, 128 - tail.length)).replace(/-+$/g, '')}${tail}`;
};

const billingRate = (
  unitPrice: string,
  billingUnit: NewModelOnboardingValues['billingUnit'],
  billingMaxQuantity: string,
): UnifiedCatalogModelOnboardRate => ({
  unit_code: billingUnit,
  unit_price: unitPrice.trim(),
  component_code: billingUnit === 'second' ? 'duration' : 'request',
  quantity_source: billingUnit === 'second' ? 'request.seconds' : 'one',
  charge_event: 'call.succeeded',
  unit_scale: 0,
  quantity_step: '0',
  max_quantity: billingUnit === 'second' ? billingMaxQuantity.trim() : '1',
  pricing_mode: 'flat',
  pricing_expr: '',
  reason_code: 'model_onboard',
});

const tokenRate = (price: string, code: string, maxTokens: string, source = `usage.${code}_tokens`): UnifiedCatalogModelOnboardRate => ({
  unit_code: 'token', unit_price: price.trim(), component_code: code,
  quantity_source: source, charge_event: 'call.succeeded',
  unit_scale: 6, quantity_step: '0', max_quantity: maxTokens.trim(),
  pricing_mode: 'flat', pricing_expr: '', reason_code: 'model_onboard',
});

export const buildModelOnboardInput = (values: NewModelOnboardingValues): UnifiedCatalogModelOnboardInput => {
  const validationError = validateNewModelOnboarding(values);
  if (validationError) throw new Error(validationError);
  const apiName = values.apiName.trim();
  const adapterCode = values.adapterCode.trim();
  const operation = downstreamOperations[values.downstreamPath];
  const modelType = modelTypeForAdapter(adapterCode, values.adapterCapability);
  const asyncImageConfig = values.capabilityConstraints.async_image;
  const taskBased = modelType === 'video' || (
    modelType === 'image'
    && adapterCode === 'openai_images'
    && Boolean(asyncImageConfig)
    && typeof asyncImageConfig === 'object'
    && !Array.isArray(asyncImageConfig)
  );
  const capabilityTag = modelType === 'image' ? 'image_generation' : modelType;
  const productCode = generatedCode([values.channelCode, apiName, 'product'], values.uniqueSuffix);
  const transportCode = generatedCode([adapterCode, apiName, 'transport'], values.uniqueSuffix);
  const capabilityConstraints = { ...values.capabilityConstraints } as Record<string, unknown>;
  if (modelType === 'llm' && values.llmCapabilities) {
    const existing = capabilityConstraints.llm;
    if (existing != null && (typeof existing !== 'object' || Array.isArray(existing))) throw new Error('LLM 能力配置须为 JSON 对象');
    const llm: Record<string, unknown> = { ...(existing as Record<string, unknown> || {}) };
    if (values.llmCapabilities.contextWindow.trim()) llm.context_window = Number(values.llmCapabilities.contextWindow.trim());
    for (const [key, enabled] of [
      ['supports_streaming', values.llmCapabilities.supportsStreaming], ['supports_tools', values.llmCapabilities.supportsTools],
      ['supports_vision', values.llmCapabilities.supportsVision], ['supports_audio', values.llmCapabilities.supportsAudio],
      ['supports_reasoning', values.llmCapabilities.supportsReasoning],
    ] as const) if (enabled) llm[key] = true;
    if (Object.keys(llm).length > 0) capabilityConstraints.llm = llm;
  }
  const payload: UnifiedCatalogModelOnboardInput = {
    expected_active_release_id: values.releaseId,
    expected_config_version: values.configVersion,
    sku: {
      model_code: apiName,
      api_name: apiName,
      display_name: values.displayName.trim(),
      description: '',
      visibility: 'visible',
      capability_tags: [capabilityTag],
      operation_code: operation.operationCode,
      contract_version: 1,
      http_method: 'POST',
      route_template: values.downstreamPath,
      normalization_version: 1,
      sku_code: generatedCode([apiName, values.variantCode, operation.operationCode], values.uniqueSuffix),
      variant_code: values.variantCode,
      delivery_mode: 'reference',
      max_results: operation.maxResults,
      idempotency_mode: 'optional',
      service_tiers: ['standard'],
    },
    product: {
      channel_id: values.channelId,
      credential_pool_id: values.poolId,
      product_code: productCode,
      vendor_model: values.vendorModel.trim(),
      capability_constraints: capabilityConstraints,
      constraints_schema_version: 1,
      adapter_code: adapterCode,
      adapter_version: values.adapterVersion,
      transport_code: transportCode,
      base_url: values.baseURL.trim().replace(/\/$/, ''),
      request_method: 'POST',
      request_path: values.requestPath.trim(),
      auth_scheme: 'bearer',
      transport_timeout_ms: taskBased ? 600000 : 30000,
      task_timeout_ms: taskBased ? 600000 : 30000,
      task_scope: taskBased ? 'task' : 'request',
      cancel_mode: 'none',
      source_url_policy: 'fixed',
      upstream_scope_kind: 'credential_pool',
      upstream_scope_key: values.poolCode,
      allowed_hosts: [],
      actions: taskBased ? [
        { action_code: 'submit', allowed_source_state: 'allocated', idempotency_mode: 'none', request_schema_version: 1, response_schema_version: 1 },
        { action_code: 'query', allowed_source_state: 'nonterminal', idempotency_mode: 'none', request_schema_version: 1, response_schema_version: 1 },
      ] : [],
      cost_plan_code: generatedCode([apiName, 'cost'], values.uniqueSuffix),
    },
    route: { priority: 100, weight: 100 },
    sell_rate: billingRate(values.sellPrice, values.billingUnit, values.billingMaxQuantity),
  };
  if (values.billingUnit !== 'token' && values.costPrice?.trim()) payload.cost_rate = billingRate(values.costPrice, values.billingUnit, values.billingMaxQuantity);
  if (values.billingUnit === 'token' && values.tokenPricing) {
    const token = values.tokenPricing;
    const cacheRead = Boolean(token.cacheReadSellPrice?.trim());
    const cacheWrite = Boolean(token.cacheWriteSellPrice?.trim());
    const inputSource = cacheWrite ? (cacheRead ? 'usage.input_no_cache' : 'usage.input_no_cache_write')
      : cacheRead ? 'usage.uncached_input_tokens' : 'usage.input_tokens';
    payload.sell_rate = tokenRate(token.inputSellPrice, 'input', token.maxInputTokens, inputSource);
    payload.additional_sell_rates = [tokenRate(token.outputSellPrice, 'output', token.maxOutputTokens)];
    if (cacheRead) payload.additional_sell_rates.push(tokenRate(token.cacheReadSellPrice!, 'cache_read', token.maxInputTokens, 'usage.cached_input_tokens'));
    if (cacheWrite) payload.additional_sell_rates.push(tokenRate(token.cacheWriteSellPrice!, 'cache_write', token.maxInputTokens, 'usage.cache_write_tokens'));
    if (token.inputCostPrice?.trim()) {
      payload.cost_rate = tokenRate(token.inputCostPrice, 'input', token.maxInputTokens);
      if (token.outputCostPrice?.trim()) payload.additional_cost_rates = [tokenRate(token.outputCostPrice, 'output', token.maxOutputTokens)];
    }
  }
  return payload;
};
