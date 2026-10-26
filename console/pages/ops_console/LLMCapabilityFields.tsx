import React from 'react';
import type { LLMCapabilityValues } from './onboardingForm';

const inputClass = 'mt-1 w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 font-mono text-sm font-normal';
const booleanFields = [
  ['supportsStreaming', '支持流式输出'], ['supportsTools', '支持工具调用'],
  ['supportsVision', '支持图像输入'], ['supportsAudio', '支持音频输入'],
  ['supportsReasoning', '支持推理标记'],
] as const;

export const LLMCapabilityFields: React.FC<{
  values: LLMCapabilityValues;
  onChange: (values: LLMCapabilityValues) => void;
  errors?: Record<string, string>;
}> = ({ values, onChange, errors = {} }) => <fieldset className="rounded-md border border-[var(--border-soft)] p-3 sm:col-span-2">
  <legend className="px-1 text-sm font-semibold">LLM 能力（可选）</legend>
  <label data-onboard-field="contextWindow" className="block max-w-xs text-sm font-semibold">上下文上限（Token）<input value={values.contextWindow} onChange={(event) => onChange({ ...values, contextWindow: event.target.value })} inputMode="numeric" maxLength={16} spellCheck={false} aria-invalid={Boolean(errors.contextWindow)} className={inputClass} placeholder="模型资料明确给出时填写" />{errors.contextWindow && <span className="mt-1 block text-xs font-normal text-rose-600">{errors.contextWindow}</span>}</label>
  <div className="mt-3 flex flex-wrap gap-x-4 gap-y-2">
    {booleanFields.map(([key, label]) => <label key={key} className="flex items-center gap-2 text-xs"><input type="checkbox" checked={values[key]} onChange={(event) => onChange({ ...values, [key]: event.target.checked })} className="h-4 w-4 accent-[var(--primary)]" />{label}</label>)}
  </div>
  <p className="mt-2 text-xs text-[var(--text-secondary)]">未填写或未勾选表示未知，不代表不支持。只记录已有资料明确确认的能力。</p>
</fieldset>;
