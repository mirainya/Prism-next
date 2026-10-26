import React, { useEffect, useRef, useState } from 'react';
import { Button, Modal, Select } from '../../components/ui';
import { fetchUnifiedCatalogModelEntries, type UnifiedCatalogModelEntry } from '../../services/unifiedGatewayApi';
import {
  fetchPriceDisplaySettings, previewPriceDisplay, savePriceDisplaySettings,
  type PriceDisplayPreview, type PriceDisplaySettings,
} from '../../services/priceDisplayApi';
import { ErrorNotice, errorMessage } from '../unified_gateway/Feedback';

const inputClass = 'w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 text-sm';

export const PriceDisplayDialog: React.FC<{
  releaseID: number;
  onClose: () => void;
}> = ({ releaseID, onClose }) => {
  const [settings, setSettings] = useState<PriceDisplaySettings | null>(null);
  const [models, setModels] = useState<UnifiedCatalogModelEntry[]>([]);
  const [modelCode, setModelCode] = useState('');
  const [preview, setPreview] = useState<PriceDisplayPreview | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const previewRequest = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const loadModels = async () => {
      const items: UnifiedCatalogModelEntry[] = [];
      for (let page = 1; ; page++) {
        const result = await fetchUnifiedCatalogModelEntries(releaseID, page, 100, controller.signal);
        items.push(...result.items);
        if (items.length >= result.total || result.items.length === 0) return items;
      }
    };
    void Promise.all([fetchPriceDisplaySettings(controller.signal), loadModels()]).then(([config, entries]) => {
      if (controller.signal.aborted) return;
      setSettings(config);
      setModels(entries);
      setModelCode(entries[0]?.model_code || '');
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) setError(errorMessage(err));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => { controller.abort(); previewRequest.current?.abort(); };
  }, [releaseID]);

  const invalidatePreview = () => {
    previewRequest.current?.abort();
    setPreviewing(false);
    setPreview(null);
    setNotice('');
  };
  const change = (next: PriceDisplaySettings) => {
    invalidatePreview();
    setSettings(next);
  };
  const override = (kind: 'model_multipliers' | 'sku_multipliers', code: string, value: string) => {
    if (!settings) return;
    const values = { ...settings[kind] };
    if (value === '') delete values[code];
    else values[code] = value;
    change({ ...settings, [kind]: values });
  };
  const selected = models.find((item) => item.model_code === modelCode);
  const publicName = selected?.skus[0]?.api_name;
  const pending = loading || saving;
  const showPreview = async () => {
    if (!settings || !publicName || pending) return;
    previewRequest.current?.abort();
    const controller = new AbortController();
    previewRequest.current = controller;
    setPreviewing(true);
    setPreview(null);
    setError('');
    try {
      const result = await previewPriceDisplay(settings, publicName, controller.signal);
      if (!controller.signal.aborted) setPreview(result);
    } catch (err: unknown) {
      if (!controller.signal.aborted) setError(errorMessage(err));
    } finally {
      if (!controller.signal.aborted) setPreviewing(false);
    }
  };
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!settings || pending) return;
    invalidatePreview();
    setSaving(true);
    setError('');
    try {
      setSettings(await savePriceDisplaySettings(settings));
      setNotice('已保存，公开价格立即生效。');
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return <Modal open title="展示定价" onClose={() => { if (!saving) onClose(); }} width="max-w-3xl">
    <form className="space-y-4" onSubmit={save}>
      <p className="text-sm text-[var(--text-secondary)]">公开价格＝独立售价 × 倍率。优先级：规格 ＞ 模型 ＞ 默认；实际扣费不变。</p>
      {error && <ErrorNotice message={error} />}
      {notice && <p role="status" className="text-sm text-[var(--primary)]">{notice}</p>}
      {loading && <p className="text-sm">正在读取配置…</p>}
      {settings && <>
        <label className="block space-y-1 text-sm font-semibold">默认倍率
          <input required aria-label="默认倍率" inputMode="decimal" pattern="[0-9]{1,6}(\.[0-9]{1,12})?" value={settings.default_multiplier} disabled={pending}
            onChange={(event) => change({ ...settings, default_multiplier: event.target.value })} className={inputClass} />
        </label>
        <div className="space-y-2 border-t border-[var(--border-soft)] pt-4">
          <p className="text-sm font-semibold">模型与规格覆盖</p>
          <Select value={modelCode} disabled={pending} options={models.map((item) => ({ value: item.model_code, label: `${item.display_name || item.model_code} · ${item.model_code}` }))}
            onChange={(value) => { invalidatePreview(); setModelCode(value); }} />
          {selected && <>
            <label className="block space-y-1 text-sm">模型倍率
              <input aria-label="模型倍率" inputMode="decimal" pattern="[0-9]{1,6}(\.[0-9]{1,12})?" value={settings.model_multipliers[modelCode] || ''} disabled={pending}
                placeholder={`继承默认 ${settings.default_multiplier}`} onChange={(event) => override('model_multipliers', modelCode, event.target.value)} className={inputClass} />
            </label>
            {selected.skus.map((sku) => <label key={sku.id} className="grid items-center gap-2 text-sm sm:grid-cols-[minmax(0,1fr)_10rem]">
              <span className="break-all">{sku.operation_code} · {sku.variant_code || sku.sku_code}</span>
              <input aria-label={`${sku.sku_code} 倍率`} inputMode="decimal" pattern="[0-9]{1,6}(\.[0-9]{1,12})?" value={settings.sku_multipliers[sku.sku_code] || ''} disabled={pending}
                placeholder={`继承 ${settings.model_multipliers[modelCode] || settings.default_multiplier}`} onChange={(event) => override('sku_multipliers', sku.sku_code, event.target.value)} className={inputClass} />
            </label>)}
            <p className="text-xs text-[var(--text-secondary)]">清空覆盖值即可恢复继承。其他模型的配置保留。</p>
          </>}
        </div>
        {preview && <PricePreview pricing={preview} />}
        <div className="flex justify-end gap-2 border-t border-[var(--border-soft)] pt-4">
          <Button type="button" variant="ghost" disabled={pending || !publicName} loading={previewing} onClick={showPreview}>预览所选模型</Button>
          <Button type="submit" loading={saving} disabled={loading}>保存</Button>
        </div>
      </>}
    </form>
  </Modal>;
};

export const PricePreview: React.FC<{ pricing: PriceDisplayPreview }> = ({ pricing }) => <section aria-label="公开价格预览" className="space-y-3 rounded-md bg-[var(--surface-muted)] p-3 text-sm">
  <h3 className="font-semibold">公开价格预览 · {pricing.currency}</h3>
  {pricing.variants.map((variant, index) => <div key={index} className="space-y-1">
    <p>{variant.operation} · {variant.service_tiers.join(' / ')}</p>
    {variant.components.map((rate, i) => <div key={i} className="space-y-1 break-words font-mono text-xs">
      <p>{rate.component}：{rate.mode === 'expression' ? rate.formula : `${rate.unit_price} / ${rate.unit_scale ? `10^${rate.unit_scale} ` : ''}${rate.unit}`}</p>
      {Object.entries(rate.tiers || {}).map(([name, tiers]) => <p key={name}>{name}：{tiers.map((tier) => `${tier.up_to ? `≤${tier.up_to}` : '其余'} → ${tier.value}`).join('；')}</p>)}
      {rate.maximum && <p>上限 {rate.maximum}</p>}
    </div>)}
  </div>)}
</section>;
