import React from 'react';
import { TokenRateSettings } from '../../services/tokenManagementApi';

export const toLocalExpiration = (value: string | null) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const pad = (part: number) => String(part).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
};
export const expirationISO = (value: string) => value ? new Date(value).toISOString() : null;
export const validTokenPolicy = (rate: string, expiration: string) =>
  /^\d+$/.test(rate) && Number(rate) <= 1000000 && (!expiration || new Date(expiration).getTime() > Date.now());

interface Props {
  rate: string;
  onRate: (value: string) => void;
  expiration: string;
  onExpiration: (value: string) => void;
  settings: TokenRateSettings | null;
  disabled?: boolean;
  prefix: string;
}
export const TokenPolicyFields: React.FC<Props> = ({ rate, onRate, expiration, onExpiration, settings, disabled, prefix }) => <>
  <div>
    <label htmlFor={`${prefix}-rate`} className="mb-1 block text-sm font-medium">每分钟调用上限</label>
    <input id={`${prefix}-rate`} type="number" min="0" max="1000000" step="1" value={rate} disabled={disabled} onChange={event => onRate(event.target.value)} className="w-full rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-3 text-sm" />
    <p className="mt-2 text-xs text-[var(--text-secondary)]">0 表示使用系统设置。{settings ? settings.enabled ? `系统上限为 ${settings.requests_per_min} 次/分钟，实际不会超过此值。` : '系统当前未启用频率限制，开启后此设置才生效。' : '系统设置尚未读取。'}</p>
  </div>
  <div>
    <label htmlFor={`${prefix}-expiration`} className="mb-1 block text-sm font-medium">到期时间</label>
    <input id={`${prefix}-expiration`} type="datetime-local" step="1" value={expiration} disabled={disabled} onChange={event => onExpiration(event.target.value)} className="w-full rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-3 text-sm" />
    <p className="mt-2 text-xs text-[var(--text-secondary)]">按本地时间填写，留空为长期有效。</p>
  </div>
</>;
