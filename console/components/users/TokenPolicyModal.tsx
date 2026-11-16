import React, { useState } from 'react';
import { Modal } from '../ui';
import { ManagedToken, TokenRateSettings, updateTokenPolicy } from '../../services/tokenManagementApi';
import { expirationISO, TokenPolicyFields, toLocalExpiration, validTokenPolicy } from './TokenPolicyFields';

interface Props { token: ManagedToken; settings: TokenRateSettings | null; onUpdated: () => void; onClose: () => void }
export const TokenPolicyModal: React.FC<Props> = ({ token, settings, onUpdated, onClose }) => {
  const [rate, setRate] = useState(String(token.rateLimit));
  const [expiration, setExpiration] = useState(toLocalExpiration(token.expiresAt));
  const [note, setNote] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const save = async () => {
    if (!validTokenPolicy(rate, expiration) || !note.trim() || saving || error) return;
    setSaving(true);
    try {
      const expiresAt = expiration === toLocalExpiration(token.expiresAt) ? token.expiresAt : expirationISO(expiration);
      await updateTokenPolicy(token, Number(rate), expiresAt, note.trim());
      onUpdated(); onClose();
    } catch (err: any) {
      setError(err.message || '保存失败，请关闭后重新查看 Key 状态');
      onUpdated();
    } finally { setSaving(false); }
  };
  return <Modal open title={`调用设置 · ${token.name}`} onClose={() => { if (!saving) onClose(); }} width="max-w-lg">
    <div className="modal-form"><div className="modal-scroll-body space-y-4">
      <p className="text-sm">Key #{token.id} · 用户 #{token.userID}</p>
      <TokenPolicyFields rate={rate} onRate={setRate} expiration={expiration} onExpiration={setExpiration} settings={settings} disabled={saving || !!error} prefix="policy" />
      {token.status === 'expired' && <p className="text-sm text-amber-700">延长有效期后，未停用的 Key 可继续调用；已停用的 Key 仍需重新启用。</p>}
      <label htmlFor="token-policy-note" className="block text-sm font-semibold">操作备注</label>
      <textarea id="token-policy-note" value={note} onChange={event => setNote(event.target.value)} rows={3} maxLength={500} disabled={saving || !!error} className="w-full rounded-lg border border-[var(--border-soft)] p-3 text-sm" />
      {error && <p role="alert" className="text-sm text-red-700">{error}。请关闭后重新查看 Key 状态。</p>}
    </div><div className="modal-footer"><button type="button" className="modal-button modal-button-secondary" disabled={saving} onClick={onClose}>关闭</button><button type="button" className="modal-button modal-button-primary" disabled={saving || !!error || !note.trim() || !validTokenPolicy(rate, expiration)} onClick={save}>{saving ? '正在保存…' : '保存设置'}</button></div></div>
  </Modal>;
};
