import React from 'react';
import type { LLMTokenPricingValues } from './onboardingForm';

const inputClass = 'mt-1 w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 font-mono text-sm font-normal';

const priceFields = [
  ['inputSellPrice', '输入', '每百万输入 Token 向用户收取的金额'],
  ['outputSellPrice', '输出', '每百万输出 Token 向用户收取的金额'],
  ['cacheReadSellPrice', '缓存读取', '留空沿用输入价格；填写 0 表示免费'],
  ['cacheWriteSellPrice', '缓存写入', '留空沿用输入价格；填写 0 表示免费'],
] as const;

const boundFields = [
  ['maxInputTokens', '输入 Token 预留上限', '预留费用时采用的输入 Token 数量上限'],
  ['maxOutputTokens', '输出 Token 预留上限', '预留费用时采用的输出 Token 数量上限'],
] as const;

export const LLMTokenPriceFields: React.FC<{
  values: LLMTokenPricingValues;
  onChange: (values: LLMTokenPricingValues) => void;
  errors?: Record<string, string>;
}> = ({ values, onChange, errors = {} }) => <>
  <fieldset className="rounded-md border border-[var(--border-soft)] p-3 sm:col-span-2">
    <legend className="px-1 text-sm font-semibold">调用价格 / 百万 Token</legend>
    <div className="grid gap-3 sm:grid-cols-2">
      {priceFields.map(([key, label, placeholder]) => <label key={key} data-onboard-field={key} className="block text-sm font-semibold">{label}<input value={values[key] || ''} onChange={(event) => onChange({ ...values, [key]: event.target.value })} inputMode="decimal" maxLength={37} spellCheck={false} aria-invalid={Boolean(errors[key])} aria-describedby={errors[key] ? `onboard-${key}-error` : undefined} className={inputClass} placeholder={placeholder} />{errors[key] && <span id={`onboard-${key}-error`} className="mt-1 block text-xs font-normal text-rose-600">{errors[key]}</span>}</label>)}
    </div>
    <p className="mt-2 text-xs text-[var(--text-secondary)]">缓存读写留空时按输入价格计费；填写后单独计费，并从普通输入中扣除对应用量。</p>
  </fieldset>
  {boundFields.map(([key, label, placeholder]) => <label key={key} data-onboard-field={key} className="block text-sm font-semibold">{label}<input value={values[key]} onChange={(event) => onChange({ ...values, [key]: event.target.value })} inputMode="numeric" maxLength={18} spellCheck={false} aria-invalid={Boolean(errors[key])} aria-describedby={`onboard-${key}-help${errors[key] ? ` onboard-${key}-error` : ''}`} className={inputClass} placeholder={placeholder} /><span id={`onboard-${key}-help`} className="mt-1 block text-xs font-normal text-[var(--text-secondary)]">用于调用前预留余额，最终按实际用量结算。</span>{errors[key] && <span id={`onboard-${key}-error`} className="mt-1 block text-xs font-normal text-rose-600">{errors[key]}</span>}</label>)}
  <p className="text-xs text-[var(--text-secondary)] sm:col-span-2">价格使用当前结算币种。Token 预留上限不限制上游请求。</p>
</>;
