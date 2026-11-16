import React, { useState } from 'react';
import { Modal } from '../ui';
import { APIRequestError } from '../../services/request';
import type { FundsAdjustment } from '../../types';

export const formatFundsAmount = (amount: string) => amount.includes('.') ? amount.replace(/0+$/, '').replace(/\.$/, '') : amount;

export const formatFundsDisplay = (amount: string) => {
  const negative = amount.startsWith('-');
  const [whole, fraction = ''] = (negative ? amount.slice(1) : amount).split('.');
  let cents = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0').slice(0, 2));
  if ((fraction[2] || '0') >= '5') cents += 1n;
  const integer = (cents / 100n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return `${negative && cents > 0n ? '-' : ''}${integer}.${(cents % 100n).toString().padStart(2, '0')}`;
};

const createRequestID = () => {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64;
  bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
};

interface Props {
  kind: 'account' | 'budget';
  targetID: string;
  name: string;
  currency?: string;
  onSubmit: (adjustment: FundsAdjustment) => Promise<unknown>;
  onClose: () => void;
  onUpdated: () => void;
}

export const FundsAdjustmentModal: React.FC<Props> = ({ kind, targetID, name, currency, onSubmit, onClose, onUpdated }) => {
  const actorID = (() => { try { return String(JSON.parse(localStorage.getItem('prism_user') || '{}').id || ''); } catch { return ''; } })();
  const storageKey = `prism-funds-operation:${actorID}:${kind}:${targetID}`;
  const [pending, setPending] = useState<FundsAdjustment | null>(() => {
    try {
      const value = JSON.parse(sessionStorage.getItem(storageKey) || 'null');
      return value && typeof value.amount === 'string' && typeof value.request_id === 'string' && typeof value.note === 'string' ? value : null;
    } catch { return null; }
  });
  const [amount, setAmount] = useState(pending?.amount || '');
  const [note, setNote] = useState(pending?.note || '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const valid = /^\d{1,20}(?:\.\d{1,18})?$/.test(amount) && /[1-9]/.test(amount) && note.trim().length > 0;
  const title = kind === 'account' ? '账户资金充值' : '增加 Key 额度';

  const submit = async () => {
    if (!valid || busy) return;
    setBusy(true);
    setError('');
    try {
      const operation = pending || { amount, note: note.trim(), request_id: createRequestID() };
      // Persist before sending, so a reload or network retry reuses the same ID.
      sessionStorage.setItem(storageKey, JSON.stringify(operation));
      setPending(operation);
      await onSubmit(operation);
      sessionStorage.removeItem(storageKey);
      onUpdated();
      onClose();
    } catch (err: unknown) {
      if (err instanceof APIRequestError && [400, 403, 404, 422].includes(err.status)) {
        sessionStorage.removeItem(storageKey);
        setPending(null);
      }
      setError(err instanceof Error ? err.message : '提交未确认，请重试');
    } finally {
      setBusy(false);
    }
  };

  return <Modal open title={title} width="max-w-md" onClose={() => { if (!busy) onClose(); }}>
    <div className="modal-form">
      <div className="modal-scroll-body space-y-4">
        <p className="font-semibold">{name}</p>
        <p className="text-sm text-[var(--text-secondary)]">{kind === 'account'
          ? '增加该用户账户的真实资金，名下 Key 共用账户资金。'
          : '增加这把 Key 的消费上限，不改变账户余额。'}</p>
        <label className="block text-sm">{kind === 'account' ? '充值金额' : '增加额度'}{currency ? `（${currency}）` : ''}
          <input aria-label={kind === 'account' ? '充值金额' : '增加额度'} inputMode="decimal" value={amount} onChange={event => setAmount(event.target.value)}
            disabled={busy || Boolean(pending)} className="mt-1 w-full rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-2 disabled:opacity-60" />
        </label>
        <label className="block text-sm">操作备注
          <textarea aria-label="操作备注" value={note} onChange={event => setNote(event.target.value)} maxLength={500} rows={2}
            disabled={busy || Boolean(pending)} className="mt-1 w-full rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-2 disabled:opacity-60" />
        </label>
        {pending && <p className="text-xs text-[var(--text-secondary)]">此笔操作已提交，重试会复用原编号，避免重复增加。金额和备注已锁定。</p>}
        {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      </div>
      <div className="modal-footer">
        <button type="button" disabled={busy} onClick={onClose} className="modal-button modal-button-secondary">关闭</button>
        <button type="button" disabled={!valid || busy} onClick={submit} className="modal-button modal-button-primary">{busy ? '提交中…' : pending ? '重试原操作' : '确认提交'}</button>
      </div>
    </div>
  </Modal>;
};
