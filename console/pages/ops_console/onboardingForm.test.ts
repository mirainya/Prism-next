import { describe, expect, it } from 'vitest';
import type { UnifiedAdapterManifest } from '../../services/unifiedGatewayApi';
import {
  buildModelOnboardInput,
  emptyLLMTokenPricing,
  emptyLLMCapabilities,
  isValidPublicModelName,
  onboardingManifestChoices,
  modelOnboardingIssue,
  type NewModelOnboardingValues,
  validateNewModelOnboarding,
} from './onboardingForm';

const baseValues = (patch: Partial<NewModelOnboardingValues> = {}): NewModelOnboardingValues => ({
  releaseId: 7,
  configVersion: 3,
  channelId: 11,
  channelCode: 'aicost',
  poolId: 12,
  poolCode: 'seedance-wholesale',
  adapterCode: 'generic',
  adapterVersion: 1,
  adapterCapability: '',
  apiName: 'seedance2.5-md-480p',
  displayName: 'Seedance 2.5 MD 480p',
  vendorModel: 'seedance2.5-md-480p',
  baseURL: 'https://api.example.com/',
  requestPath: '/v1/videos/generations',
  downstreamPath: '/v1/videos/generations',
  variantCode: 'default',
  capabilityConstraints: { adapter: { profile: 'json_task_v1' } },
  billingUnit: 'request',
  billingMaxQuantity: '',
  sellPrice: '1.25',
  costPrice: '0.80',
  uniqueSuffix: 'test1',
  ...patch,
});

const tokenValues = (patch: Partial<NewModelOnboardingValues> = {}): NewModelOnboardingValues => baseValues({
  adapterCode: 'openai_chat',
  apiName: 'model-a-2',
  vendorModel: 'model-a',
  requestPath: '/v1/chat/completions',
  downstreamPath: '/v1/chat/completions',
  capabilityConstraints: {},
  billingUnit: 'token',
  sellPrice: '',
  costPrice: '',
  tokenPricing: {
    inputSellPrice: '2', outputSellPrice: '8', inputCostPrice: '1', outputCostPrice: '4',
    maxInputTokens: '128000', maxOutputTokens: '32768',
  },
  ...patch,
});

describe('model onboarding form', () => {
  it.each([
    ['', '', 'usage.input_tokens', []],
    ['0', '', 'usage.uncached_input_tokens', ['usage.cached_input_tokens']],
    ['', '3', 'usage.input_no_cache_write', ['usage.cache_write_tokens']],
    ['0.2', '3', 'usage.input_no_cache', ['usage.cached_input_tokens', 'usage.cache_write_tokens']],
  ])('uses disjoint input meters for cache prices %s / %s', (cacheReadSellPrice, cacheWriteSellPrice, inputSource, cacheSources) => {
    const values = tokenValues();
    values.tokenPricing = { ...values.tokenPricing!, inputCostPrice: undefined, outputCostPrice: undefined, cacheReadSellPrice, cacheWriteSellPrice };
    const payload = buildModelOnboardInput(values);
    expect(payload.sell_rate.quantity_source).toBe(inputSource);
    expect(payload.additional_sell_rates!.slice(1).map(rate => rate.quantity_source)).toEqual(cacheSources);
    expect(payload.cost_rate).toBeUndefined();
    if (cacheReadSellPrice === '0') expect(payload.additional_sell_rates![1].unit_price).toBe('0');
  });

  it('identifies each invalid reservation field with an actionable message', () => {
    const values = tokenValues();
    values.tokenPricing = { ...values.tokenPricing!, maxInputTokens: '' };
    expect(modelOnboardingIssue(values)).toEqual({ field: 'maxInputTokens', message: '请填写输入 Token 预留上限' });
    values.tokenPricing.maxInputTokens = '128000';
    values.tokenPricing.maxOutputTokens = '1.5';
    expect(modelOnboardingIssue(values)).toEqual({ field: 'maxOutputTokens', message: '输出 Token 预留上限请输入大于零的整数，最多 18 位' });
  });

  it('rejects malformed cache prices without treating empty prices as free', () => {
    const values = tokenValues();
    values.tokenPricing = { ...values.tokenPricing!, cacheWriteSellPrice: '-1' };
    expect(modelOnboardingIssue(values)?.field).toBe('cacheWriteSellPrice');
  });

  it('creates independent input and output token rates in one onboarding request', () => {
    const payload = buildModelOnboardInput(tokenValues());
    expect(payload.sell_rate).toMatchObject({
      unit_code: 'token', unit_price: '2', unit_scale: 6, component_code: 'input',
      quantity_source: 'usage.input_tokens', max_quantity: '128000', quantity_step: '0',
      charge_event: 'call.succeeded', pricing_mode: 'flat', pricing_expr: '',
    });
    expect(payload.cost_rate).toMatchObject({ unit_price: '1', quantity_source: 'usage.input_tokens', unit_scale: 6 });
    expect(payload.additional_sell_rates).toEqual([{
      unit_code: 'token', unit_price: '8', unit_scale: 6, component_code: 'output',
      quantity_source: 'usage.output_tokens', max_quantity: '32768', quantity_step: '0',
      charge_event: 'call.succeeded', pricing_mode: 'flat', pricing_expr: '', reason_code: 'model_onboard',
    }]);
    expect(payload.additional_cost_rates).toEqual([{
      ...payload.additional_sell_rates![0], unit_price: '4',
    }]);
    expect(payload.product.vendor_model).toBe('model-a');
    expect(payload.sku.api_name).toBe('model-a-2');
    expect(payload.product.capability_constraints).toEqual({});
    expect(payload.product.actions).toEqual([]);
  });

  it('stores explicitly known LLM capabilities without turning omitted fields into false', () => {
    const payload = buildModelOnboardInput(tokenValues({ llmCapabilities: {
      ...emptyLLMCapabilities(), contextWindow: '128000', supportsStreaming: true, supportsTools: true,
    } }));
    expect(payload.product.capability_constraints).toEqual({ llm: {
      context_window: 128000, supports_streaming: true, supports_tools: true,
    } });
    const unknown = buildModelOnboardInput(tokenValues({ llmCapabilities: emptyLLMCapabilities() }));
    expect(unknown.product.capability_constraints).toEqual({});
  });

  it('rejects malformed LLM context bounds', () => {
    for (const value of ['0', '1e6', '1'.repeat(19), '9007199254740992']) {
      expect(validateNewModelOnboarding(tokenValues({ llmCapabilities: { ...emptyLLMCapabilities(), contextWindow: value } }))).toContain('上下文上限');
    }
  });

  it('merges declared JSON capabilities and preserves precise context bounds', () => {
    const payload = buildModelOnboardInput(tokenValues({
      capabilityConstraints: { llm: { supports_tools: false, supports_audio: true }, adapter: { profile: 'unchanged' } },
      llmCapabilities: { ...emptyLLMCapabilities(), contextWindow: '9007199254740991', supportsStreaming: true },
    }));
    expect(payload.product.capability_constraints).toEqual({
      adapter: { profile: 'unchanged' },
      llm: { context_window: 9007199254740991, supports_tools: false, supports_audio: true, supports_streaming: true },
    });
    expect(() => buildModelOnboardInput(tokenValues({ capabilityConstraints: { llm: [] }, llmCapabilities: emptyLLMCapabilities() }))).toThrow('LLM 能力配置须为 JSON 对象');
  });

  it.each([
    ['openai_chat', '/v1/chat/completions'],
    ['openai_responses', '/v1/responses'],
    ['anthropic_messages', '/v1/messages'],
    ['google_generate_content', '/v1/chat/completions'],
    ['volcengine_responses_v3', '/v1/responses'],
  ])('uses the same token pricing for %s', (adapterCode, downstreamPath) => {
    const payload = buildModelOnboardInput(tokenValues({ adapterCode, downstreamPath }));
    expect(payload.sku.capability_tags).toEqual(['llm']);
    expect(payload.sell_rate.quantity_source).toBe('usage.input_tokens');
    expect(payload.additional_sell_rates![0].quantity_source).toBe('usage.output_tokens');
  });

  it('retains precise prices and large token bounds as strings without applying the display coefficient', () => {
    const payload = buildModelOnboardInput(tokenValues({ tokenPricing: {
      inputSellPrice: '0.000000000000000001', outputSellPrice: '123.456789123456789123',
      inputCostPrice: '0', outputCostPrice: '0.0000001',
      maxInputTokens: '999999999999999999', maxOutputTokens: '9007199254740993',
    } }));
    expect(payload.sell_rate.unit_price).toBe('0.000000000000000001');
    expect(payload.additional_sell_rates![0].unit_price).toBe('123.456789123456789123');
    expect(payload.cost_rate.unit_price).toBe('0');
    expect(payload.sell_rate.max_quantity).toBe('999999999999999999');
    expect(payload.additional_cost_rates![0].max_quantity).toBe('9007199254740993');
  });

  it('never substitutes missing token prices or reservation bounds with zero', () => {
    expect(emptyLLMTokenPricing()).toEqual({
      inputSellPrice: '', outputSellPrice: '', cacheReadSellPrice: '', cacheWriteSellPrice: '',
      maxInputTokens: '', maxOutputTokens: '',
    });
    expect(validateNewModelOnboarding(tokenValues({ tokenPricing: undefined }))).toContain('请填写');
    for (const key of ['inputSellPrice', 'outputSellPrice'] as const) {
      for (const value of ['', '-1', '1e-6', 'NaN', '0.1234567891234567891']) {
        const candidate = tokenValues();
        candidate.tokenPricing = { ...candidate.tokenPricing!, [key]: value };
        expect(() => buildModelOnboardInput(candidate)).toThrow('价格');
      }
    }
    for (const key of ['maxInputTokens', 'maxOutputTokens'] as const) {
      for (const value of ['', '0', '-1', '1.5', '1e6', '9999999999999999999']) {
        const candidate = tokenValues();
        candidate.tokenPricing = { ...candidate.tokenPricing!, [key]: value };
        expect(() => buildModelOnboardInput(candidate)).toThrow('预留上限');
      }
    }
  });

  it('rejects token billing for image and video onboarding', () => {
    expect(validateNewModelOnboarding(tokenValues({ adapterCode: 'openai_images' }))).toContain('LLM');
    expect(validateNewModelOnboarding(tokenValues({ adapterCode: 'generic' }))).toContain('LLM');
  });

  it('keeps two public identities independent without changing the actual upstream model', () => {
    const first = buildModelOnboardInput(baseValues({ apiName: 'model-a', vendorModel: 'model-a' }));
    const second = buildModelOnboardInput(baseValues({ apiName: 'model-a-2', vendorModel: 'model-a', poolId: 99, poolCode: 'key-2', sellPrice: '2.50', costPrice: '1.50' }));
    expect(first.sku.api_name).toBe('model-a');
    expect(second.sku.api_name).toBe('model-a-2');
    expect(second.sku.model_code).toBe('model-a-2');
    expect(second.product.vendor_model).toBe('model-a');
    expect(second.product.credential_pool_id).toBe(99);
    expect(second.sell_rate.unit_price).toBe('2.50');
    expect(second.cost_rate.unit_price).toBe('1.50');
  });

  it('validates public identities in UTF-8 bytes before requesting suggestions', () => {
    expect(isValidPublicModelName('a' + '图'.repeat(42))).toBe(true);
    expect(isValidPublicModelName('a' + '图'.repeat(43))).toBe(false);
    expect(isValidPublicModelName('a'.repeat(128))).toBe(true);
    expect(isValidPublicModelName('a'.repeat(129))).toBe(false);
    expect(isValidPublicModelName('a\n')).toBe(false);
  });
  it('uses adapter manifest paths and variants', () => {
    const manifest: UnifiedAdapterManifest = {
      adapter: 'seedance',
      adapter_version: 1,
      protocol: 'seedance',
      has_manifest: true,
      downstream_paths: ['/v1/videos/generations'],
      variants: [
        { code: 'official', display: '官方', models: [], param_hints: {} },
        { code: 'seedance25', display: 'Seedance 2.5', models: [], param_hints: {} },
      ],
    };
    expect(onboardingManifestChoices('seedance', manifest)).toEqual({
      downstreamPaths: ['/v1/videos/generations'],
      variants: [
        { code: 'official', label: '官方' },
        { code: 'seedance25', label: 'Seedance 2.5' },
      ],
    });
  });

  it('provides the generic video fallback', () => {
    expect(onboardingManifestChoices('generic', {
      adapter: 'generic', adapter_version: 1, protocol: 'video_generation', has_manifest: false,
    })).toEqual({
      downstreamPaths: ['/v1/videos/generations'],
      variants: [{ code: 'default', label: '默认' }],
    });
  });

  it('builds a complete task-based onboarding request billed by second', () => {
    const payload = buildModelOnboardInput(baseValues({
      billingUnit: 'second',
      billingMaxQuantity: '12',
    }));
    expect(payload.sku).toMatchObject({
      model_code: 'seedance2.5-md-480p',
      api_name: 'seedance2.5-md-480p',
      operation_code: 'videos.generate',
      route_template: '/v1/videos/generations',
      variant_code: 'default',
      capability_tags: ['video'],
    });
    expect(payload.product).toMatchObject({
      channel_id: 11,
      credential_pool_id: 12,
      adapter_code: 'generic',
      task_scope: 'task',
      upstream_scope_kind: 'credential_pool',
      upstream_scope_key: 'seedance-wholesale',
      base_url: 'https://api.example.com',
    });
    expect(payload.product.actions.map((item) => item.action_code)).toEqual(['submit', 'query']);
    expect(payload.additional_sell_rates).toBeUndefined();
    expect(payload.additional_cost_rates).toBeUndefined();
    expect(payload.sell_rate).toMatchObject({
      unit_code: 'second',
      unit_price: '1.25',
      component_code: 'duration',
      quantity_source: 'request.seconds',
      max_quantity: '12',
      pricing_mode: 'flat',
    });
    expect(payload.cost_rate).toMatchObject({
      unit_code: 'second',
      unit_price: '0.80',
      component_code: 'duration',
      quantity_source: 'request.seconds',
      max_quantity: '12',
      pricing_mode: 'flat',
    });
    expect(payload.product.product_code.length).toBeLessThanOrEqual(128);
  });

  it('builds a synchronous model billed by request with a fixed quantity bound', () => {
    const payload = buildModelOnboardInput(baseValues({
      adapterCode: 'openai_chat',
      apiName: 'gpt-new',
      displayName: 'GPT New',
      vendorModel: 'gpt-new-upstream',
      requestPath: '/v1/chat/completions',
      downstreamPath: '/v1/chat/completions',
      capabilityConstraints: {},
      billingUnit: 'request',
      billingMaxQuantity: '86400',
    }));
    expect(payload.sku.operation_code).toBe('chat.completions');
    expect(payload.sku.capability_tags).toEqual(['llm']);
    expect(payload.product.task_scope).toBe('request');
    expect(payload.product.actions).toEqual([]);
    expect(payload.additional_sell_rates).toBeUndefined();
    expect(payload.additional_cost_rates).toBeUndefined();
    expect(payload.sell_rate).toMatchObject({
      unit_code: 'request',
      component_code: 'request',
      quantity_source: 'one',
      max_quantity: '1',
      pricing_mode: 'flat',
    });
    expect(payload.cost_rate).toMatchObject({
      unit_code: 'request',
      unit_price: '0.80',
      component_code: 'request',
      quantity_source: 'one',
      max_quantity: '1',
      pricing_mode: 'flat',
    });
  });

  it('builds a synchronous image model with the routable image capability', () => {
    const payload = buildModelOnboardInput(baseValues({
      adapterCode: 'openai_images',
      adapterCapability: 'image',
      apiName: 'gpt-image-public',
      displayName: 'GPT Image',
      vendorModel: 'gpt-image-1',
      requestPath: '/v1/images/generations',
      downstreamPath: '/v1/images/generations',
      capabilityConstraints: { sizes: ['1024x1024'] },
    }));

    expect(payload.sku.capability_tags).toEqual(['image_generation']);
    expect(payload.product.task_scope).toBe('request');
    expect(payload.product.actions).toEqual([]);
  });

  it('builds an asynchronous image model with submit and query actions', () => {
    const payload = buildModelOnboardInput(baseValues({
      adapterCode: 'openai_images',
      adapterCapability: 'image',
      apiName: 'gpt-image-async',
      displayName: 'GPT Image Async',
      vendorModel: 'gpt-image-2',
      requestPath: '/v1/images/generations?async=true',
      downstreamPath: '/v1/images/generations',
      capabilityConstraints: {
        async_image: {
          profile: 'json_image_task_v1',
          submit: { method: 'POST', path: '/v1/images/generations?async=true' },
          poll: { method: 'GET', path: '/v1/tasks/{task_id}' },
        },
      },
    }));

    expect(payload.sku.capability_tags).toEqual(['image_generation']);
    expect(payload.product.task_scope).toBe('task');
    expect(payload.product.actions.map((item) => item.action_code)).toEqual(['submit', 'query']);
  });

  it('omits unknown cost rather than creating a zero-priced cost rate', () => {
    const values = baseValues({ costPrice: '' });
    expect(validateNewModelOnboarding(values)).toBe('');
    expect(buildModelOnboardInput(values).cost_rate).toBeUndefined();
    const token = tokenValues({ tokenPricing: { ...emptyLLMTokenPricing(), inputSellPrice: '2', outputSellPrice: '8', maxInputTokens: '128000', maxOutputTokens: '32768' } });
    expect(buildModelOnboardInput(token).cost_rate).toBeUndefined();
    expect(buildModelOnboardInput(token).additional_cost_rates).toBeUndefined();
  });

  it('rejects invalid second billing bounds and non-video models', () => {
    for (const billingMaxQuantity of ['', '0', '1.5', '86401']) {
      expect(validateNewModelOnboarding(baseValues({
        billingUnit: 'second',
        billingMaxQuantity,
      }))).toContain('1-86400');
    }
    expect(validateNewModelOnboarding(baseValues({
      adapterCode: 'openai_chat',
      adapterCapability: '',
      requestPath: '/v1/chat/completions',
      downstreamPath: '/v1/chat/completions',
      billingUnit: 'second',
      billingMaxQuantity: '60',
    }))).toContain('视频模型');
  });

  it('accepts actual AICost public model names', () => {
    for (const apiName of ['seedance2.5-9图', 'seedance2.5-10图', 'seedance2.0-480p-100%']) {
      expect(validateNewModelOnboarding(baseValues({ apiName }))).toBe('');
    }
  });

  it('rejects a missing billing unit, invalid public identity, and prices', () => {
    expect(validateNewModelOnboarding(baseValues({ billingUnit: '' }))).toContain('计费单位');
    expect(validateNewModelOnboarding(baseValues({ apiName: '模型' }))).toContain('公开调用名');
    expect(validateNewModelOnboarding(baseValues({ apiName: 'seedance2.5 图' }))).toContain('公开调用名');
    expect(validateNewModelOnboarding(baseValues({ apiName: `a${'图'.repeat(43)}` }))).toContain('公开调用名');
    expect(validateNewModelOnboarding(baseValues({ sellPrice: '0' }))).toContain('价格');
    expect(validateNewModelOnboarding(baseValues({ costPrice: '-1' }))).toContain('成本');
  });
});
