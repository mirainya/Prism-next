import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, Pencil, Power, RefreshCw, Save } from 'lucide-react';
import { Button, Modal, Select } from '../../components/ui';
import {
  createAvailabilitySource,
  fetchAvailabilitySourceOptions,
  fetchAvailabilitySources,
  transitionAvailabilitySource,
  updateAvailabilitySource,
  type AvailabilitySourceContract,
  type UnifiedAvailabilitySourceOptions,
} from '../../services/unifiedCatalogSourceApi';
import type { UnifiedAvailabilitySource } from '../../services/unifiedGatewayApi';
import { ErrorNotice, errorMessage } from './Feedback';
import { ProtocolDefinitionsDialog } from './ProtocolDefinitionsDialog';

const inputClass = 'mt-1 w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 text-sm font-normal';

export const AvailabilitySourcesDialog: React.FC<{
  onClose: () => void;
  onChanged: () => void;
}> = ({ onClose, onChanged }) => {
  const [options, setOptions] = useState<UnifiedAvailabilitySourceOptions | null>(null);
  const [items, setItems] = useState<UnifiedAvailabilitySource[]>([]);
  const [editing, setEditing] = useState<UnifiedAvailabilitySource | null>(null);
  const [channelID, setChannelID] = useState('');
  const [credentialID, setCredentialID] = useState('');
  const [contractCode, setContractCode] = useState<AvailabilitySourceContract | ''>('');
  const [sourceCode, setSourceCode] = useState('');
  const [baseURL, setBaseURL] = useState('');
  const [timeout, setTimeout] = useState('15000');
  const [statusFilter, setStatusFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [definitionID, setDefinitionID] = useState('');
  const [protocolsOpen, setProtocolsOpen] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError('');
    try {
      const [sourceOptions, result] = await Promise.all([
        fetchAvailabilitySourceOptions(signal),
        fetchAvailabilitySources(1, 100, statusFilter, signal),
      ]);
      if (signal?.aborted) return;
      setOptions(sourceOptions);
      setItems(result.items);
      setContractCode((current) => current || sourceOptions.contracts[0]?.code || '');
      setChannelID((current) => current || String(sourceOptions.channels[0]?.id || ''));
    } catch (err: unknown) {
      if (!signal?.aborted) setError(errorMessage(err));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [statusFilter]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const channelCredentials = useMemo(
    () => (options?.credentials || []).filter((item) => String(item.channel_id) === channelID),
    [options, channelID],
  );

  const resetForm = () => {
    setEditing(null);
    setSourceCode('');
    setDefinitionID('');
    setCredentialID('');
    setBaseURL('');
    setTimeout('15000');
  };

  const edit = (source: UnifiedAvailabilitySource) => {
    setEditing(source);
    setChannelID(String(source.channel_id));
    setContractCode(source.contract_code as AvailabilitySourceContract);
    setDefinitionID(String(source.protocol_definition_id || ''));
    setCredentialID(String(source.credential_id));
    setSourceCode(source.source_code);
    setBaseURL(source.base_url);
    setTimeout(String(source.request_timeout_ms));
    setError('');
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (pending || loading || !contractCode || (contractCode === 'declarative_availability_v1' && !definitionID) || !channelCredentials.some((item) => String(item.id) === credentialID)) return;
    setPending(true);
    setError('');
    try {
      const connection = {
        credential_id: Number(credentialID),
        base_url: baseURL,
        request_timeout_ms: Number(timeout),
        contract_code: contractCode,
        protocol_definition_id: contractCode === 'declarative_availability_v1' ? Number(definitionID) : 0,
      };
      if (editing) {
        await updateAvailabilitySource(editing, connection);
      } else {
        await createAvailabilitySource({
          ...connection,
          channel_id: Number(channelID),
          source_code: sourceCode,
          contract_code: contractCode,
        });
      }
      resetForm();
      await load();
      onChanged();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  const toggle = async (source: UnifiedAvailabilitySource) => {
    if (pending) return;
    setPending(true);
    setError('');
    try {
      await transitionAvailabilitySource(source);
      await load();
      onChanged();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  if (protocolsOpen) return <ProtocolDefinitionsDialog onClose={() => { setProtocolsOpen(false); void load(); }} />;

  return (
    <Modal open title="可用性来源" onClose={onClose} width="max-w-5xl">
      <div className="space-y-5">
        {error && <ErrorNotice message={error} />}
        <form onSubmit={(event) => void submit(event)} className="grid gap-3 border-b border-[var(--border-soft)] pb-5 md:grid-cols-2">
          <label className="block text-sm font-semibold">渠道
            <Select
              value={channelID}
              onChange={(value) => { setChannelID(value); setCredentialID(''); }}
              options={editing
                ? [{ value: String(editing.channel_id), label: `${editing.channel_name} · ${editing.channel_code}` }]
                : (options?.channels || []).map((item) => ({ value: String(item.id), label: `${item.name} · ${item.code}` }))}
              disabled={pending || loading || !!editing || !options?.channels.length}
              className="mt-1"
            />
          </label>
          <label className="block text-sm font-semibold">监控类型
            <Select
              value={contractCode}
              onChange={(value) => { setContractCode(value as AvailabilitySourceContract); setDefinitionID(''); }}
              options={(options?.contracts || []).map((item) => ({ value: item.code, label: item.name }))}
              disabled={pending || loading || !options?.contracts.length}
              className="mt-1"
            />
          </label>
          {contractCode === 'declarative_availability_v1' && <label className="block text-sm font-semibold md:col-span-2">协议配置
            <Select value={definitionID} onChange={setDefinitionID} options={(options?.protocol_definitions || []).map((item) => ({ value: String(item.id), label: `${item.display_name} · v${item.protocol_version}` }))} placeholder="选择启用的协议版本" disabled={pending || loading} className="mt-1" />
          </label>}
          <label className="block text-sm font-semibold">读取凭据
            <Select
              value={credentialID}
              onChange={setCredentialID}
              options={channelCredentials.map((item) => ({ value: String(item.id), label: item.code }))}
              placeholder="选择已有凭据"
              disabled={pending || loading || !channelCredentials.length}
              className="mt-1"
            />
          </label>
          <label className="block text-sm font-semibold">来源标识
            <input className={inputClass} disabled={pending || !!editing} required maxLength={128} pattern="[a-z][a-z0-9_.-]{0,127}" value={sourceCode} onChange={(event) => setSourceCode(event.target.value)} />
          </label>
          <label className="block text-sm font-semibold md:col-span-2">上游地址
            <input className={inputClass} disabled={pending} type="url" required maxLength={2048} placeholder="https://api.example.com" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} />
          </label>
          <label className="block text-sm font-semibold">请求超时（毫秒）
            <input className={inputClass} disabled={pending} type="number" required min={1000} max={120000} step={1} value={timeout} onChange={(event) => setTimeout(event.target.value)} />
          </label>
          <div className="flex items-end justify-end gap-2">
            {editing && <Button type="button" variant="ghost" disabled={pending} onClick={resetForm}>取消编辑</Button>}
            <Button type="submit" loading={pending} disabled={loading || !channelID || !channelCredentials.some((item) => String(item.id) === credentialID) || !contractCode || (contractCode === 'declarative_availability_v1' && !definitionID) || !sourceCode || !baseURL}>
              <Save size={15} />{editing ? '保存修改' : '新增来源'}
            </Button>
          </div>
        </form>

        <div className="flex items-center justify-between gap-3">
          <h4 className="flex items-center gap-2 text-sm font-bold"><Activity size={16} />已配置来源</h4>
          <div className="flex items-center gap-2">
            <Button type="button" variant="ghost" disabled={pending} onClick={() => setProtocolsOpen(true)}>协议配置</Button>
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              disabled={pending}
              options={[{ value: '', label: '全部状态' }, { value: 'active', label: '启用' }, { value: 'disabled', label: '停用' }]}
              className="min-w-32"
            />
            <Button type="button" variant="ghost" title="刷新来源列表" onClick={() => void load()} disabled={loading || pending}><RefreshCw size={15} /></Button>
          </div>
        </div>
        {loading ? (
          <div role="status" className="py-8 text-center text-sm text-[var(--text-secondary)]">正在读取</div>
        ) : (
          <div className="max-h-72 overflow-auto rounded-md border border-[var(--border-soft)]">
            <table className="w-full min-w-[720px] text-left text-sm">
              <thead className="sticky top-0 bg-[var(--surface-muted)] text-xs text-[var(--text-secondary)]">
                <tr><th className="px-3 py-2">渠道 / 来源</th><th className="px-3 py-2">监控类型</th><th className="px-3 py-2">地址</th><th className="px-3 py-2">凭据</th><th className="px-3 py-2">状态</th><th className="px-3 py-2" /></tr>
              </thead>
              <tbody className="divide-y divide-[var(--border-soft)]">
                {items.map((source) => (
                  <tr key={source.id}>
                    <td className="px-3 py-2"><span className="block font-medium">{source.channel_name}</span><code className="text-xs text-[var(--text-secondary)]">{source.source_code}</code></td>
                    <td className="px-3 py-2">{options?.contracts.find((item) => item.code === source.contract_code)?.name || source.contract_code}</td>
                    <td className="max-w-64 truncate px-3 py-2 text-xs" title={source.base_url}>{source.base_url}</td>
                    <td className="px-3 py-2">{source.credential_code}</td>
                    <td className="px-3 py-2">{source.status === 'active' ? '启用' : '停用'}</td>
                    <td className="px-3 py-2"><div className="flex justify-end">
                      <Button type="button" variant="ghost" title="编辑来源" disabled={pending} onClick={() => edit(source)}><Pencil size={15} /></Button>
                      <Button type="button" variant="ghost" title={source.status === 'active' ? '停用来源' : '启用来源'} disabled={pending || !!editing} onClick={() => void toggle(source)}><Power size={15} /></Button>
                    </div></td>
                  </tr>
                ))}
                {!items.length && <tr><td colSpan={6} className="py-10 text-center text-sm text-[var(--text-secondary)]">暂无监控来源</td></tr>}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Modal>
  );
};
