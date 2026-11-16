import React, { useCallback, useEffect, useState } from 'react';
import { Button, Modal, Select } from '../../components/ui';
import { createProtocolDefinition, fetchProtocolDefinitions, readProtocolDefinition, transitionProtocolDefinition, type UnifiedProtocolDefinition } from '../../services/unifiedCatalogSourceApi';
import { ErrorNotice, errorMessage } from './Feedback';

const inputClass = 'mt-1 w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 text-sm font-normal';
export const ProtocolDefinitionsDialog: React.FC<{ onClose: () => void; initialKind?: 'availability' | 'catalog_discovery' }> = ({ onClose, initialKind = 'availability' }) => {
  const [kind, setKind] = useState<'availability' | 'catalog_discovery'>(initialKind);
  const [items, setItems] = useState<UnifiedProtocolDefinition[]>([]);
  const [code, setCode] = useState('');
  const [version, setVersion] = useState(1);
  const [name, setName] = useState('');
  const [raw, setRaw] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await fetchProtocolDefinitions(kind, signal);
      if (!signal?.aborted) setItems(result.items);
    } catch (err) { if (!signal?.aborted) setError(errorMessage(err)); }
  }, [kind]);
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);
  const copy = async (item: UnifiedProtocolDefinition) => {
    setPending(true); setError('');
    try {
      const full = await readProtocolDefinition(item.id);
      setCode(item.protocol_code);
      setName(item.display_name);
      setVersion(Math.max(...items.filter((entry) => entry.protocol_code === item.protocol_code).map((entry) => entry.protocol_version)) + 1);
      setRaw(JSON.stringify(full.definition, null, 2));
    } catch (err) { setError(errorMessage(err)); } finally { setPending(false); }
  };
  const save = async (event: React.FormEvent) => {
    event.preventDefault(); setPending(true); setError('');
    try {
      await createProtocolDefinition({ protocol_code: code, protocol_version: version, protocol_kind: kind, display_name: name, definition: JSON.parse(raw) });
      setCode(''); setVersion(1); setName(''); setRaw('');
      await load();
    } catch (err) { setError(errorMessage(err)); } finally { setPending(false); }
  };
  const toggle = async (item: UnifiedProtocolDefinition) => {
    setPending(true); setError('');
    try { await transitionProtocolDefinition(item); await load(); }
    catch (err) { setError(errorMessage(err)); } finally { setPending(false); }
  };
  return <Modal open title="协议配置" onClose={onClose} width="max-w-5xl">
    <div className="space-y-4">
      {error && <ErrorNotice message={error} />}
      <Select value={kind} onChange={(value) => { setKind(value as typeof kind); setCode(''); setName(''); setRaw(''); setVersion(1); setItems([]); }} options={[{ value: 'availability', label: '可用性' }, { value: 'catalog_discovery', label: '模型与价格发现' }]} disabled={pending} />
      <p className="text-sm text-[var(--text-secondary)]">修改配置创建新版本；保存后在来源中选择该版本。凭据由来源单独绑定。</p>
      <form onSubmit={(event) => void save(event)} className="grid gap-3 md:grid-cols-3">
        <label className="text-sm font-semibold">协议标识<input className={inputClass} value={code} onChange={(event) => setCode(event.target.value)} pattern="[a-z][a-z0-9_.-]{0,127}" maxLength={128} required disabled={pending} /></label>
        <label className="text-sm font-semibold">版本<input className={inputClass} type="number" min={1} max={4294967295} step={1} value={version} onChange={(event) => setVersion(Number(event.target.value))} required disabled={pending} /></label>
        <label className="text-sm font-semibold">名称<input className={inputClass} value={name} onChange={(event) => setName(event.target.value)} maxLength={128} required disabled={pending} /></label>
        <label className="text-sm font-semibold md:col-span-3">配置 JSON<textarea className={`${inputClass} min-h-48 font-mono`} value={raw} onChange={(event) => setRaw(event.target.value)} required disabled={pending} spellCheck={false} /></label>
        <div className="md:col-span-3 flex justify-end"><Button type="submit" loading={pending}>保存新版本</Button></div>
      </form>
      <div className="max-h-64 overflow-auto border border-[var(--border-soft)] rounded-md">
        <table className="w-full text-left text-sm">
          <thead><tr><th className="px-3 py-2">名称 / 标识</th><th className="px-3 py-2">版本</th><th className="px-3 py-2">状态</th><th /></tr></thead>
          <tbody>{items.map((item) => <tr key={item.id} className="border-t border-[var(--border-soft)]">
            <td className="px-3 py-2">{item.display_name}<code className="block text-xs text-[var(--text-secondary)]">{item.protocol_code}</code></td>
            <td className="px-3 py-2">{item.protocol_version}</td><td className="px-3 py-2">{item.status === 'active' ? '启用' : '停用'}</td>
            <td className="px-3 py-2"><Button type="button" variant="ghost" disabled={pending} onClick={() => void copy(item)}>查看 / 新版本</Button><Button type="button" variant="ghost" disabled={pending} onClick={() => void toggle(item)}>{item.status === 'active' ? '停用' : '启用'}</Button></td>
          </tr>)}</tbody>
        </table>
      </div>
    </div>
  </Modal>;
};
