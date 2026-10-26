import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Braces, Save } from 'lucide-react';
import { Button, Modal, SegmentedControl, Select } from '../../components/ui';
import {
  createUnifiedActiveProduct,
  fetchUnifiedAdapterManifest,
  fetchUnifiedCatalogModelEntries,
  fetchUnifiedCatalogOptions,
  onboardUnifiedCatalogModel,
  suggestUnifiedCatalogModelName,
  type UnifiedAdapterManifest,
  type UnifiedCatalogModelEntry,
  type UnifiedCatalogOptions,
  type UnifiedCatalogRelease,
} from '../../services/unifiedGatewayApi';
import type { UnifiedChannel, UnifiedPool } from '../../services/unifiedChannelApi';
import { ErrorNotice, errorMessage } from '../unified_gateway/Feedback';
import type { ModelDiscoveryPreset } from './catalogSnapshotForm';
import { LLMTokenPriceFields } from './LLMTokenPriceFields';
import { LLMCapabilityFields } from './LLMCapabilityFields';
import {
  buildModelOnboardInput,
  emptyLLMTokenPricing,
  emptyLLMCapabilities,
  isValidPublicModelName,
  modelTypeForAdapter,
  onboardingManifestChoices,
  modelOnboardingIssue,
} from './onboardingForm';

const JsonEditor = React.lazy(() => import('../../components/ui/JsonEditor'));

const inputClass = 'mt-1 w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 text-sm font-normal';

const FieldError: React.FC<{ field: string; errors: Record<string, string> }> = ({ field, errors }) => errors[field]
  ? <span className="mt-1 block text-xs font-normal text-rose-600">{errors[field]}</span> : null;

const adapterLabels: Record<string, string> = {
  anthropic_messages: 'Anthropic Messages',
  generic: '通用 JSON 映射',
  google_generate_content: 'Google Generate Content',
  openai_chat: 'OpenAI Chat Completions',
  openai_images: 'OpenAI Images',
  openai_responses: 'OpenAI Responses',
  seedance: 'Seedance',
  volcengine_responses_v3: '火山 Responses V3',
};

const requestPaths: Record<string, string> = {
  anthropic_messages: '/v1/messages',
  generic: '/tasks',
  google_generate_content: '/v1beta/models/{model}:generateContent',
  openai_chat: '/v1/chat/completions',
  openai_images: '/v1/images/generations',
  openai_responses: '/v1/responses',
  seedance: '/api/v3/contents/generations/tasks',
  volcengine_responses_v3: '/api/v3/responses',
};

const genericMapping = (path: string) => JSON.stringify({
  adapter: {
    profile: 'json_task_v1',
    auth_location: 'header',
    auth_key: 'Authorization',
    auth_prefix: 'Bearer ',
    submit: { enabled: true, method: 'POST', path },
    poll: { enabled: true, method: 'GET', path: `${path.replace(/\/$/, '')}/{task_id}` },
    request: {
      fields: {
        model: 'model', prompt: 'prompt', resolution: 'resolution', ratio: 'ratio',
        duration: 'duration', audio: 'generate_audio', task_mode: 'task_mode',
      },
      params_mode: 'merge_missing',
    },
    response: {
      task_id_paths: ['data.id', 'id'],
      status_paths: ['data.status', 'status'],
      video_url_paths: ['data.result.video_url', 'data.video_url', 'result.video_url', 'video_url'],
      error_paths: ['data.error.message', 'data.message', 'error.message', 'message'],
      status_map: {
        queued: 'submitted', pending: 'submitted', running: 'tracking', processing: 'tracking',
        succeeded: 'completed', completed: 'completed', failed: 'failed', canceled: 'cancelled',
      },
      submit_default_status: 'submitted',
      poll_default_status: 'tracking',
      unknown_status: 'tracking',
    },
  },
}, null, 2);

const identityPart = (value: string) => value.toLowerCase()
  .replace(/[^a-z0-9]+/g, '-')
  .replace(/^-+|-+$/g, '') || 'model';

const loadModelEntries = async (releaseID: number, signal: AbortSignal) => {
  const items: UnifiedCatalogModelEntry[] = [];
  let page = 1;
  while (true) {
    const result = await fetchUnifiedCatalogModelEntries(releaseID, page, 100, signal);
    items.push(...result.items);
    if (items.length >= result.total || result.items.length === 0) return items;
    page += 1;
  }
};

export const AddUpstreamModelDialog: React.FC<{
  channel: UnifiedChannel;
  release: UnifiedCatalogRelease;
  pools: UnifiedPool[];
  initial?: ModelDiscoveryPreset;
  onClose: () => void;
  onSaved: () => void;
}> = ({ channel, release, pools, initial, onClose, onSaved }) => {
  const [mode, setMode] = useState<'existing' | 'new'>('new');
  const [options, setOptions] = useState<UnifiedCatalogOptions | null>(null);
  const [models, setModels] = useState<UnifiedCatalogModelEntry[]>([]);
  const [manifest, setManifest] = useState<UnifiedAdapterManifest | null>(null);
  const [manifestLoading, setManifestLoading] = useState(false);
  const [manifestError, setManifestError] = useState('');
  const [poolID, setPoolID] = useState(String(initial?.poolId || pools.find((pool) => pool.status === 'active')?.id || ''));
  const [adapter, setAdapter] = useState('');
  const [vendorModel, setVendorModel] = useState(initial?.vendorModel || '');
  const [baseURL, setBaseURL] = useState(initial?.baseURL || '');
  const [requestPath, setRequestPath] = useState('/v1/chat/completions');
  const [mapping, setMapping] = useState('{}');
  const [skuIDs, setSkuIDs] = useState<number[]>([]);
  const [apiName, setAPIName] = useState('');
  const [nameSuggestion, setNameSuggestion] = useState<{ baseName: string } | null>(
    initial && isValidPublicModelName(initial.vendorModel) ? { baseName: initial.vendorModel } : null,
  );
  const [nameSuggestionLoading, setNameSuggestionLoading] = useState(false);
  const [nameSuggestionError, setNameSuggestionError] = useState('');
  const [associationConfirmed, setAssociationConfirmed] = useState(false);
  const [displayName, setDisplayName] = useState('');
  const [downstreamPath, setDownstreamPath] = useState('');
  const [variantCode, setVariantCode] = useState('default');
  const [billingUnit, setBillingUnit] = useState<'' | 'request' | 'second' | 'token'>('');
  const [billingMaxQuantity, setBillingMaxQuantity] = useState('');
  const [sellPrice, setSellPrice] = useState('');
  const [tokenPricing, setTokenPricing] = useState(emptyLLMTokenPricing);
  const [llmCapabilities, setLLMCapabilities] = useState(emptyLLMCapabilities);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const formRef = useRef<HTMLFormElement>(null);

  const showFieldError = (field: string, message: string) => {
    setError(message);
    setFieldErrors({ [field]: message });
    requestAnimationFrame(() => {
      const element = formRef.current?.querySelector<HTMLElement>(`[data-onboard-field="${field}"]`);
      element?.scrollIntoView({ block: 'center' });
      element?.querySelector<HTMLElement>('input,button,textarea,[contenteditable="true"]')?.focus({ preventScroll: true });
    });
  };

  const clearFieldError = (field: string) => {
    setError('');
    setFieldErrors(current => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  };

  useEffect(() => {
    if (mode !== 'new' || !nameSuggestion) {
      setNameSuggestionLoading(false);
      return;
    }
    const controller = new AbortController();
    setNameSuggestionLoading(true);
    setNameSuggestionError('');
    suggestUnifiedCatalogModelName(nameSuggestion.baseName, controller.signal).then((result) => {
      if (!controller.signal.aborted) setAPIName(result.api_name);
    }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setNameSuggestionError(errorMessage(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setNameSuggestionLoading(false);
    });
    return () => controller.abort();
  }, [mode, nameSuggestion]);

  useEffect(() => { setAssociationConfirmed(false); }, [poolID, adapter, skuIDs]);

  const suggestName = () => {
    const baseName = vendorModel.trim() || apiName.trim();
    if (!isValidPublicModelName(baseName)) {
      setNameSuggestionError('请先填写有效的公开调用名或上游模型名');
      return;
    }
    setNameSuggestion({ baseName });
  };

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      fetchUnifiedCatalogOptions(release.id, controller.signal),
      loadModelEntries(release.id, controller.signal),
    ]).then(([nextOptions, nextModels]) => {
      if (controller.signal.aborted) return;
      setOptions(nextOptions);
      setModels(nextModels);
      const preferred = channel.model_types.includes('video')
        ? 'generic'
        : channel.model_types.includes('image') ? 'openai_images' : 'openai_chat';
      const initial = nextOptions.adapters.find((item) => item.code === preferred) || nextOptions.adapters[0];
      if (initial) {
        setAdapter(`${initial.code}@${initial.version}`);
        const path = requestPaths[initial.code] || '/';
        setRequestPath(path);
        setMapping(initial.code === 'generic' ? genericMapping(path) : '{}');
      }
    }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(errorMessage(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [channel.model_types, release.id]);

  const selectedAdapter = options?.adapters.find((item) => `${item.code}@${item.version}` === adapter);
  const selectedPool = pools.find((pool) => String(pool.id) === poolID);
  const targetType = modelTypeForAdapter(selectedAdapter?.code || 'openai_chat', manifest?.capability);
  const compatibleModels = useMemo(() => models.filter((model) => model.model_type === targetType), [models, targetType]);
  const availableSKUs = compatibleModels.flatMap((model) => model.skus.map((sku) => ({ ...sku, modelName: model.display_name || model.model_code })));
  const manifestChoices = useMemo(
    () => onboardingManifestChoices(selectedAdapter?.code || '', manifest),
    [manifest, selectedAdapter?.code],
  );

  useEffect(() => {
    if (mode !== 'new' || !selectedAdapter) {
      setManifest(null);
      setManifestError('');
      setManifestLoading(false);
      return;
    }
    const controller = new AbortController();
    setManifest(null);
    setManifestError('');
    setManifestLoading(true);
    fetchUnifiedAdapterManifest(selectedAdapter.code, selectedAdapter.version, controller.signal)
      .then((nextManifest) => {
        if (controller.signal.aborted) return;
        const choices = onboardingManifestChoices(selectedAdapter.code, nextManifest);
        setManifest(nextManifest);
        setDownstreamPath((current) => choices.downstreamPaths.includes(current) ? current : (choices.downstreamPaths[0] || ''));
        setVariantCode((current) => choices.variants.some((item) => item.code === current)
          ? current
          : choices.variants.length === 1 ? choices.variants[0].code : '');
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setManifestError(errorMessage(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setManifestLoading(false);
      });
    return () => controller.abort();
  }, [mode, selectedAdapter?.code, selectedAdapter?.version]);

  const changeAdapter = (value: string) => {
    const next = options?.adapters.find((item) => `${item.code}@${item.version}` === value);
    const path = requestPaths[next?.code || ''] || '/';
    const choices = onboardingManifestChoices(next?.code || '', null);
    setAdapter(value);
    setRequestPath(path);
    setMapping(next?.code === 'generic' ? genericMapping(path) : '{}');
    setSkuIDs([]);
    setManifest(null);
    setDownstreamPath(choices.downstreamPaths[0] || '');
    setVariantCode(next?.code === 'generic' ? 'default' : '');
    setBillingUnit('');
    setBillingMaxQuantity('');
    setTokenPricing(emptyLLMTokenPricing());
    setLLMCapabilities(emptyLLMCapabilities());
  };

  const changeRequestPath = (value: string) => {
    setMapping((current) => selectedAdapter?.code === 'generic' && current === genericMapping(requestPath)
      ? genericMapping(value)
      : current);
    setRequestPath(value);
  };

  const toggleSKU = (id: number) => setSkuIDs((current) => current.includes(id)
    ? current.filter((value) => value !== id)
    : [...current, id]);

  const selectedVariant = manifest?.variants?.find((item) => item.code === variantCode);
  const suggestedMaxSeconds = selectedVariant?.param_hints?.duration?.max || '';

  const changeVariant = (value: string) => {
    setVariantCode(value);
    if (billingUnit !== 'second') return;
    const maxSeconds = manifest?.variants?.find((item) => item.code === value)?.param_hints?.duration?.max || '';
    setBillingMaxQuantity(maxSeconds);
  };

  const changeBillingUnit = (value: string) => {
    const next = value === 'second' ? 'second' : value === 'token' ? 'token' : value === 'request' ? 'request' : '';
    setBillingUnit(next);
    setBillingMaxQuantity(next === 'second' ? suggestedMaxSeconds : '');
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (saving || nameSuggestionLoading) return;
    if (!selectedAdapter || !selectedPool || selectedPool.status !== 'active') {
      showFieldError(!selectedAdapter ? 'adapter' : 'poolID', '请选择有效的接入格式和 Key 池');
      return;
    }
    if (mode === 'existing' && skuIDs.length === 0) {
      showFieldError('skuIDs', '请至少选择一个已有对外模型');
      return;
    }
    if (mode === 'existing' && !associationConfirmed) {
      showFieldError('associationConfirmed', '关联已有模型前，须确认价格、权限和服务规格一致');
      return;
    }
    const modelName = vendorModel.trim();
    const endpoint = baseURL.trim().replace(/\/$/, '');
    const path = requestPath.trim();
    if (!modelName || !endpoint || !path.startsWith('/')) {
      showFieldError(!modelName ? 'vendorModel' : !endpoint ? 'baseURL' : 'requestPath', '请填写上游模型、上游地址和请求路径');
      return;
    }
    try {
      const parsedURL = new URL(endpoint);
      if (!['http:', 'https:'].includes(parsedURL.protocol) || !parsedURL.hostname || parsedURL.username || parsedURL.password) throw new Error();
    } catch {
      showFieldError('baseURL', '上游地址必须是有效的 HTTP 或 HTTPS 地址');
      return;
    }
    let parsedMapping: Record<string, unknown>;
    try {
      parsedMapping = JSON.parse(mapping) as Record<string, unknown>;
      if (!parsedMapping || typeof parsedMapping !== 'object' || Array.isArray(parsedMapping)) throw new Error();
    } catch {
      showFieldError('mapping', 'JSON 映射必须是有效对象');
      return;
    }
    const stamp = Date.now().toString(36);
    const onboardingValues = {
      releaseId: release.id,
      configVersion: release.config_version,
      channelId: channel.id,
      channelCode: channel.channel_code,
      poolId: selectedPool.id,
      poolCode: selectedPool.pool_code,
      adapterCode: selectedAdapter.code,
      adapterVersion: selectedAdapter.version,
      adapterCapability: manifest?.capability || '',
      apiName,
      displayName,
      vendorModel: modelName,
      baseURL: endpoint,
      requestPath: path,
      downstreamPath,
      variantCode,
      capabilityConstraints: parsedMapping,
      billingUnit,
      billingMaxQuantity,
      sellPrice,
      tokenPricing,
      llmCapabilities,
      uniqueSuffix: stamp,
    };
    if (mode === 'new') {
      const validationError = modelOnboardingIssue(onboardingValues);
      if (validationError) {
        showFieldError(validationError.field, validationError.message);
        return;
      }
    }
    setSaving(true);
    setError('');
    setFieldErrors({});
    try {
      if (mode === 'new') {
        await onboardUnifiedCatalogModel(buildModelOnboardInput(onboardingValues));
      } else {
        const stem = `${identityPart(channel.channel_code)}-${identityPart(modelName)}`;
        const productCode = `${stem.slice(0, 118 - stamp.length)}-${stamp}`;
        const transportCode = `${identityPart(selectedAdapter.code).slice(0, 108)}-${stamp}`;
        // Declaration-driven async image transports use the same task
        // lifecycle as video products. Without this branch, associating an
        // existing public image SKU with a provider's /async endpoint would be
        // persisted as a request-scoped product and never enqueue polling.
        const asyncImage = parsedMapping.async_image;
        const taskBased = targetType === 'video' || (
          targetType === 'image'
          && selectedAdapter.code === 'openai_images'
          && Boolean(asyncImage)
          && typeof asyncImage === 'object'
          && !Array.isArray(asyncImage)
        );
        await createUnifiedActiveProduct({
          expected_active_release_id: release.id,
          expected_config_version: release.config_version,
          channel_id: channel.id,
          credential_pool_id: selectedPool.id,
          product_code: productCode,
          vendor_model: modelName,
          capability_constraints: parsedMapping,
          constraints_schema_version: 1,
          adapter_code: selectedAdapter.code,
          adapter_version: selectedAdapter.version,
          transport_code: transportCode,
          base_url: endpoint,
          request_method: 'POST',
          request_path: path,
          auth_scheme: 'bearer',
          transport_timeout_ms: taskBased ? 600000 : 30000,
          task_timeout_ms: taskBased ? 600000 : 30000,
          task_scope: taskBased ? 'task' : 'request',
          cancel_mode: 'none',
          source_url_policy: 'fixed',
          upstream_scope_kind: 'credential_pool',
          upstream_scope_key: selectedPool.pool_code,
          allowed_hosts: [],
          actions: taskBased ? [
            { action_code: 'submit', allowed_source_state: 'allocated', idempotency_mode: 'none', request_schema_version: 1, response_schema_version: 1 },
            { action_code: 'query', allowed_source_state: 'nonterminal', idempotency_mode: 'none', request_schema_version: 1, response_schema_version: 1 },
          ] : [],
          cost_plan_code: 'standard',
          routes: skuIDs.map((sku_id) => ({ sku_id, priority: 100, weight: 100 })),
        });
      }
      onSaved();
    } catch (reason: unknown) {
      setError(errorMessage(reason));
    } finally {
      setSaving(false);
    }
  };

  return <Modal open title="接入上游模型" width="max-w-3xl" onClose={() => { if (!saving) onClose(); }}>
    <form ref={formRef} className="modal-form" onSubmit={submit} onChangeCapture={event => {
      const field = (event.target as HTMLElement).closest<HTMLElement>('[data-onboard-field]')?.dataset.onboardField;
      if (field) clearFieldError(field);
    }}>
      <div className="modal-scroll-body space-y-4">
      <SegmentedControl
        ariaLabel="模型接入方式"
        value={mode}
        onChange={(value) => { setMode(value); setError(''); setFieldErrors({}); }}
        options={[
          { value: 'existing', label: '关联已有模型' },
          { value: 'new', label: '新建公开模型' },
        ]}
      />
      {initial && <p className="text-xs text-[var(--text-secondary)]">来自列表快照 #{initial.snapshotId}。仅带入上游模型名、地址与此 Key 的分组；公开名、价格与能力仍需补全。</p>}
      {initial?.priceReference && <p className="text-xs text-[var(--text-secondary)]">上游价格资料（仅供参考）：{initial.priceReference}</p>}
      <div className="grid gap-3 sm:grid-cols-2">
        {mode === 'new' && <>
          <div><label data-onboard-field="apiName" className="block text-sm font-semibold">公开调用名<input autoFocus value={apiName} onChange={(event) => { setNameSuggestion(null); setNameSuggestionError(''); setAPIName(event.target.value); }} maxLength={128} spellCheck={false} className={`${inputClass} font-mono`} placeholder="下游请求中 model 字段使用的名称" /><FieldError field="apiName" errors={fieldErrors} /></label><div className="mt-1 flex items-center gap-2"><Button type="button" variant="ghost" loading={nameSuggestionLoading} disabled={saving} onClick={suggestName}>生成独立名称</Button><span className="text-xs text-[var(--text-secondary)]">重名使用 -2、-3；上游名不变</span></div></div>
          <label data-onboard-field="displayName" className="block text-sm font-semibold">显示名<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={128} className={inputClass} placeholder="模型列表和页面展示的名称" /><FieldError field="displayName" errors={fieldErrors} /></label>
        </>}
        <label data-onboard-field="poolID" className="block text-sm font-semibold">Key 池<Select value={poolID} onChange={value => { clearFieldError("poolID"); setPoolID(value); }} disabled={loading || saving} className="mt-1" options={pools.filter((pool) => pool.status === 'active').map((pool) => ({ value: String(pool.id), label: pool.display_name }))} /><FieldError field="poolID" errors={fieldErrors} /></label>
        <label data-onboard-field="adapter" className="block text-sm font-semibold">接入格式<Select value={adapter} onChange={value => { clearFieldError("adapter"); changeAdapter(value); }} disabled={loading || saving} className="mt-1" options={(options?.adapters || []).map((item) => ({ value: `${item.code}@${item.version}`, label: adapterLabels[item.code] || item.code }))} /><FieldError field="adapter" errors={fieldErrors} /></label>
        <label data-onboard-field="vendorModel" className="block text-sm font-semibold">上游模型<input autoFocus={mode === 'existing'} value={vendorModel} onChange={(event) => { setNameSuggestion(null); setNameSuggestionError(''); setVendorModel(event.target.value); setAssociationConfirmed(false); }} maxLength={255} spellCheck={false} className={`${inputClass} font-mono`} placeholder="发给上游的模型名" /><FieldError field="vendorModel" errors={fieldErrors} /></label>
        <label data-onboard-field="baseURL" className="block text-sm font-semibold">上游地址<input value={baseURL} onChange={(event) => setBaseURL(event.target.value)} maxLength={500} spellCheck={false} className={`${inputClass} font-mono`} placeholder="上游 API 的基础地址，含协议及所需前缀" /><FieldError field="baseURL" errors={fieldErrors} /></label>
        <label data-onboard-field="requestPath" className="block text-sm font-semibold sm:col-span-2">上游请求路径<input value={requestPath} onChange={(event) => changeRequestPath(event.target.value)} maxLength={255} spellCheck={false} className={`${inputClass} font-mono`} /><FieldError field="requestPath" errors={fieldErrors} /></label>
        {mode === 'new' && <>
          <label data-onboard-field="downstreamPath" className="block text-sm font-semibold">下游入口<Select value={downstreamPath} onChange={value => { clearFieldError("downstreamPath"); setDownstreamPath(value); }} disabled={manifestLoading || manifestChoices.downstreamPaths.length === 0} className="mt-1" options={manifestChoices.downstreamPaths.map((path) => ({ value: path, label: path }))} />{!manifestLoading && manifestChoices.downstreamPaths.length === 0 && <span className="mt-1 block text-xs font-normal text-[var(--text-secondary)]">该接入格式未声明下游入口</span>}<FieldError field="downstreamPath" errors={fieldErrors} /></label>
          <label data-onboard-field="variantCode" className="block text-sm font-semibold">模型规格<Select value={variantCode} onChange={value => { clearFieldError("variantCode"); changeVariant(value); }} placeholder="请选择规格" disabled={manifestLoading} className="mt-1" options={manifestChoices.variants.map((item) => ({ value: item.code, label: item.label }))} /><FieldError field="variantCode" errors={fieldErrors} /></label>
          <label data-onboard-field="billingUnit" className="block text-sm font-semibold">计费单位<Select value={billingUnit} onChange={value => { clearFieldError("billingUnit"); changeBillingUnit(value); }} placeholder="请选择计费单位" className="mt-1" options={[
            { value: 'request', label: '按次' },
            ...(targetType === 'video' ? [{ value: 'second', label: '按请求秒数' }] : []),
            ...(targetType === 'llm' ? [{ value: 'token', label: '按 Token' }] : []),
          ]} /><FieldError field="billingUnit" errors={fieldErrors} /></label>
          {billingUnit === 'second' && <label data-onboard-field="billingMaxQuantity" className="block text-sm font-semibold">单次最多秒数<input value={billingMaxQuantity} onChange={(event) => setBillingMaxQuantity(event.target.value)} inputMode="numeric" maxLength={5} spellCheck={false} className={`${inputClass} font-mono`} placeholder="预留费用时采用的单次秒数上限" /><FieldError field="billingMaxQuantity" errors={fieldErrors} /></label>}
          {billingUnit === 'token' ? <LLMTokenPriceFields values={tokenPricing} onChange={setTokenPricing} errors={fieldErrors} /> : <>
            <label data-onboard-field="sellPrice" className="block text-sm font-semibold">调用价格 / {billingUnit === 'second' ? '秒' : '次'}<input value={sellPrice} onChange={(event) => setSellPrice(event.target.value)} inputMode="decimal" maxLength={37} spellCheck={false} className={`${inputClass} font-mono`} placeholder={billingUnit === 'second' ? '每秒向用户收取的金额' : '每次调用向用户收取的金额'} /><FieldError field="sellPrice" errors={fieldErrors} /></label>
          </>}
        </>}
      </div>

      {mode === 'existing' && <fieldset data-onboard-field="skuIDs" className="rounded-md border border-[var(--border-soft)] p-3">
        <legend className="px-1 text-sm font-semibold">对外模型</legend>
        <FieldError field="skuIDs" errors={fieldErrors} />
        {loading ? <p className="py-3 text-xs text-[var(--text-secondary)]">正在读取</p> : availableSKUs.length === 0 ? <p className="py-3 text-xs text-[var(--text-secondary)]">当前目录没有可匹配的模型</p> : <div className="grid max-h-48 gap-1 overflow-y-auto sm:grid-cols-2">{availableSKUs.map((sku) => <label key={sku.id} className="flex min-w-0 items-center gap-2 rounded-md px-2 py-2 text-xs hover:bg-[var(--surface-muted)]"><input type="checkbox" checked={skuIDs.includes(sku.id)} onChange={() => toggleSKU(sku.id)} className="h-4 w-4 accent-[var(--primary)]" /><span className="min-w-0"><strong className="block truncate">{sku.modelName}</strong><code className="block truncate text-[10px] font-normal text-[var(--text-secondary)]">{sku.api_name} · {sku.route_template}</code></span></label>)}</div>}
        <label data-onboard-field="associationConfirmed" className="mt-2 flex items-center gap-2 text-xs"><input type="checkbox" checked={associationConfirmed} onChange={(event) => setAssociationConfirmed(event.target.checked)} className="h-4 w-4 accent-[var(--primary)]" />确认与所选模型的价格、权限和服务规格一致，允许共用公开模型<FieldError field="associationConfirmed" errors={fieldErrors} /></label>
      </fieldset>}

      {mode === 'new' && targetType === 'llm' && <LLMCapabilityFields values={llmCapabilities} onChange={setLLMCapabilities} errors={fieldErrors} />}

      <label data-onboard-field="mapping" className="block text-sm font-semibold"><span className="inline-flex items-center gap-1.5"><Braces size={14} />{targetType === 'video' ? '上下游 JSON 映射' : '参数与能力配置'}</span><div className="mt-1"><React.Suspense fallback={<div className="grid h-56 place-items-center rounded-md border border-[var(--border-soft)] text-xs text-[var(--text-secondary)]">正在加载编辑器</div>}><JsonEditor value={mapping} onChange={value => { clearFieldError('mapping'); setMapping(value); }} height="14rem" /></React.Suspense></div><FieldError field="mapping" errors={fieldErrors} /></label>

      </div>
      <div className="modal-footer shrink-0" style={{ flexDirection: 'column', alignItems: 'stretch' }}>
        {(error || manifestError || nameSuggestionError) && <ErrorNotice message={error || manifestError || nameSuggestionError} />}
        <div className="flex justify-end gap-2"><Button type="button" variant="ghost" onClick={onClose} disabled={saving}>取消</Button><Button type="submit" loading={saving} disabled={loading || (mode === 'new' && (manifestLoading || nameSuggestionLoading))}><Save size={14} />{mode === 'new' ? '新建并生效' : '保存并生效'}</Button></div>
      </div>
    </form>
  </Modal>;
};
