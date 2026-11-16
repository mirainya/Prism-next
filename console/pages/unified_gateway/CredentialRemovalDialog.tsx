import React, { useEffect, useRef, useState } from 'react';
import { RefreshCw, Trash2 } from 'lucide-react';
import { Button, Modal } from '../../components/ui';
import { fetchCredentialRemovalImpact, removeManagedCredential, type CredentialRemovalImpact } from '../../services/unifiedCredentialApi';
import type { UnifiedCredential } from '../../services/unifiedGatewayApi';
import { ErrorNotice, errorMessage } from './Feedback';

export const CredentialRemovalDialog: React.FC<{
  item: UnifiedCredential;
  readOnly: boolean;
  onClose: () => void;
  onSaved: () => void;
}> = ({ item, readOnly, onClose, onSaved }) => {
  const [impact, setImpact] = useState<CredentialRemovalImpact | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const changed = useRef(false);
  const locked = useRef(false);
  const refresh = async () => {
    if (locked.current) return;
    locked.current = true; setPending(true); setError('');
    try { setImpact(await fetchCredentialRemovalImpact(item)); }
    catch (reason) { setImpact(null); setError(errorMessage(reason)); }
    finally { locked.current = false; setPending(false); }
  };
  useEffect(() => {
    let active = true;
    setPending(true);
    fetchCredentialRemovalImpact(item).then(
      value => { if (active) setImpact(value); },
      reason => { if (active) setError(errorMessage(reason)); },
    ).finally(() => { if (active) setPending(false); });
    return () => { active = false; };
  }, [item.id]);
  const remove = async () => {
    if (locked.current || readOnly || !impact || impact.secret_removed) return;
    locked.current = true; setPending(true); setError('');
    try {
      const result = await removeManagedCredential({ ...item, config_version: impact.config_version });
      setImpact(result); changed.current = true;
    } catch (reason) {
      setError(errorMessage(reason)); setImpact(null);
    } finally { locked.current = false; setPending(false); }
  };
  const close = () => { if (!pending) { if (changed.current) onSaved(); else onClose(); } };
  const waiting = impact && (impact.active_slots > 0 || impact.unresolved_attempts > 0 || impact.control_runs > 0 || impact.pending_deliveries > 0);
  return <Modal open onClose={close} title="移除 API Key" width="max-w-xl">
    <div className="space-y-4">
      <p className="text-sm">先停止新调用，已有任务结束后清除密钥。模型配置、价格、账务和历史记录保留。</p>
      {error && <ErrorNotice message={error} />}
      {impact && <>
        <div className="rounded-lg border border-[var(--border-soft)] p-3 text-sm">
          <p className="font-semibold">{impact.secret_removed ? '密钥已移除' : impact.status === 'active' ? '即将停止使用此 Key' : '此 Key 已停止新调用'}</p>
          <dl className="mt-2 grid grid-cols-2 gap-2 text-xs">
            <div>进行中调用：{impact.unresolved_attempts}</div><div>占用中的请求：{impact.active_slots}</div>
            <div>目录等控制任务：{impact.control_runs}</div><div>待交付结果：{impact.pending_deliveries}</div>
          </dl>
          {waiting && <p className="mt-2 text-xs text-[var(--text-secondary)]">任务或待核实调用结束后，点击刷新，再完成移除。</p>}
        </div>
        <div className="text-sm"><p className="font-semibold">关联模型（{impact.models.length}）</p>
          {impact.models.length === 0 ? <p className="mt-2 text-xs text-[var(--text-secondary)]">没有关联模型</p> : <ul className="mt-2 max-h-48 space-y-1 overflow-y-auto text-xs">
            {impact.models.map(model => <li key={model.api_name} className="flex justify-between gap-3"><code>{model.api_name}</code><span>{model.unavailable ? '失去全部线路，将不可调用' : '保留其他可用线路'}</span></li>)}
          </ul>}
        </div>
      </>}
      <div className="flex flex-wrap justify-between gap-2 border-t border-[var(--border-soft)] pt-3">
        <Button type="button" variant="ghost" disabled={pending} onClick={() => void refresh()}><RefreshCw size={14} />刷新</Button>
        <div className="flex gap-2">
          <Button type="button" variant="ghost" disabled={pending} onClick={close}>关闭</Button>
          {!impact?.secret_removed && <Button type="button" variant="danger" loading={pending} disabled={readOnly || !impact || impact.status !== 'active' && Boolean(waiting)} onClick={() => void remove()}><Trash2 size={14} />{impact?.status === 'active' ? '确认移除 Key' : '完成移除'}</Button>}
        </div>
      </div>
    </div>
  </Modal>;
};
