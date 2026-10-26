import React, { useEffect, useState } from 'react';
import { RefreshCw, Search, Settings2 } from 'lucide-react';
import { Button, Modal, Pagination, Select } from '../../components/ui';
import {
  fetchCatalogDiscoveries, fetchCatalogDiscoveryItems, fetchProtocolDefinitions,
  type UnifiedCatalogDiscovery, type UnifiedCatalogDiscoveryItem, type UnifiedCatalogSource, type UnifiedProtocolDefinition,
} from '../../services/unifiedCatalogSourceApi';
import type { UnifiedCredential } from '../../services/unifiedGatewayApi';
import { ErrorNotice, errorMessage } from '../unified_gateway/Feedback';
import { ProtocolDefinitionsDialog } from '../unified_gateway/ProtocolDefinitionsDialog';
import { formatCredentialDisplayName, formatOpsDate } from './presentation';
import {
  catalogRunStateLabel, catalogPriceFacts, discoveryModelPreset, loadCredentialCatalogSources, pullCredentialModelList,
  newCatalogSourceFields, validateCatalogSourceForm, type ModelDiscoveryPreset,
} from './catalogSnapshotForm';

const inputClass = 'mt-1 w-full rounded-md border border-[var(--border-soft)] bg-[var(--surface-card-solid)] px-3 py-2 text-sm font-normal';
const pendingRun = (run?: UnifiedCatalogDiscovery) => run?.state === 'scheduled' || run?.state === 'running';

export const CredentialCatalogDialog: React.FC<{
  credential: UnifiedCredential;
  displayName?: string;
  connectionBaseURLs?: string[];
  releaseId: number;
  canEdit: boolean;
  onClose: () => void;
  onSelect: (preset: ModelDiscoveryPreset) => void;
}> = ({ credential, displayName, connectionBaseURLs = [], releaseId, canEdit, onClose, onSelect }) => {
  const [sources, setSources] = useState<UnifiedCatalogSource[]>([]);
  const [definitions, setDefinitions] = useState<UnifiedProtocolDefinition[]>([]);
  const [sourceChoice, setSourceChoice] = useState('');
  const [loading, setLoading] = useState(true);
  const [connectionRevision, setConnectionRevision] = useState(0);
  const [baseURL, setBaseURL] = useState(() => newCatalogSourceFields(connectionBaseURLs, 0).base_url);
  const [group, setGroup] = useState('default');
  const [timeout, setTimeoutValue] = useState('15000');
  const [definitionId, setDefinitionId] = useState('');
  const [protocolsOpen, setProtocolsOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [history, setHistory] = useState<UnifiedCatalogDiscovery[]>([]);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyRevision, setHistoryRevision] = useState(0);
  const [selectedRunId, setSelectedRunId] = useState<number | null>(null);
  const [items, setItems] = useState<UnifiedCatalogDiscoveryItem[]>([]);
  const [itemTotal, setItemTotal] = useState(0);
  const [itemPage, setItemPage] = useState(1);
  const [itemsLoading, setItemsLoading] = useState(false);
  const [query, setQuery] = useState('');
  const selectedSource = sources.find(source => String(source.id) === sourceChoice);
  const selectedRun = history.find(run => run.id === selectedRunId);
  const snapshotId = selectedRun?.snapshot_id;
  const snapshotSource = sources.find(source => source.id === selectedRun?.source_id);
  const newSource = sourceChoice === 'new';
  const writable = canEdit && credential.status === 'active';

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    Promise.all([loadCredentialCatalogSources(credential.id, controller.signal), fetchProtocolDefinitions('catalog_discovery', controller.signal)])
      .then(([nextSources, result]) => {
        if (controller.signal.aborted) return;
        const activeDefinitions = result.items.filter(item => item.status === 'active');
        setSources(nextSources);
        setDefinitions(activeDefinitions);
        setSourceChoice(current => current === 'new' || nextSources.some(source => String(source.id) === current)
          ? current : String(nextSources.find(source => source.status === 'active')?.id || nextSources[0]?.id || 'new'));
        // Never automatically choose another upstream's login protocol.
        setDefinitionId(current => activeDefinitions.some(item => String(item.id) === current) ? current
          : String(activeDefinitions.find(item => item.protocol_code === 'standard_model_catalog')?.id || ''));
      })
      .catch(reason => { if (!controller.signal.aborted) setError(errorMessage(reason)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [credential.id, connectionRevision]);

  useEffect(() => {
    if (loading) return;
    const controller = new AbortController();
    setHistoryLoading(true);
    fetchCatalogDiscoveries(releaseId, historyPage, 20, controller.signal, { credentialId: credential.id, sourceId: selectedSource?.id })
      .then(result => {
        if (controller.signal.aborted) return;
        setHistory(result.items); setHistoryTotal(result.total);
        setSelectedRunId(current => result.items.some(run => run.id === current) ? current : result.items[0]?.id || null);
      })
      .catch(reason => { if (!controller.signal.aborted) setError(errorMessage(reason)); })
      .finally(() => { if (!controller.signal.aborted) setHistoryLoading(false); });
    return () => controller.abort();
  }, [credential.id, releaseId, selectedSource?.id, historyPage, historyRevision, loading]);

  // This polls Prism's stored run state only. It never refreshes the upstream.
  useEffect(() => {
    if (historyLoading || !pendingRun(history[0])) return;
    const timer = window.setTimeout(() => setHistoryRevision(current => current + 1), 2000);
    return () => window.clearTimeout(timer);
  }, [history, historyLoading]);

  useEffect(() => { setItemPage(1); setQuery(''); }, [snapshotId]);
  useEffect(() => {
    const controller = new AbortController();
    setItems([]); setItemTotal(0);
    if (!snapshotId) { setItemsLoading(false); return () => controller.abort(); }
    setItemsLoading(true);
    fetchCatalogDiscoveryItems(snapshotId, itemPage, 50, controller.signal, query.trim())
      .then(result => { if (!controller.signal.aborted) { setItems(result.items); setItemTotal(result.total); } })
      .catch(reason => { if (!controller.signal.aborted) setError(errorMessage(reason)); })
      .finally(() => { if (!controller.signal.aborted) setItemsLoading(false); });
    return () => controller.abort();
  }, [snapshotId, itemPage, query]);

  const pull = async (event: React.FormEvent) => {
    event.preventDefault();
    if (pending || loading || !writable) return;
    const fields = { base_url: baseURL.trim().replace(/\/+$/, ''), external_group: group.trim(), request_timeout_ms: Number(timeout), protocol_definition_id: Number(definitionId) };
    if (newSource) {
      const message = validateCatalogSourceForm(fields);
      if (message) { setError(message); return; }
    } else if (!selectedSource || selectedSource.status !== 'active' || !definitions.some(item => item.id === selectedSource.protocol_definition_id)) {
      setError('此拉取配置或协议不可用'); return;
    }
    setPending(true); setError('');
    try {
      const result = await pullCredentialModelList(credential.id, releaseId, selectedSource?.id || null, fields);
      if (newSource) setSources(await loadCredentialCatalogSources(credential.id));
      setSourceChoice(String(result.sourceId)); setHistoryPage(1); setSelectedRunId(result.runId);
      setHistoryRevision(current => current + 1);
    } catch (reason) { setError(errorMessage(reason)); }
    finally { setPending(false); }
  };

  const select = (item: UnifiedCatalogDiscoveryItem) => {
    if (!snapshotId || !snapshotSource || !writable) return;
    try { onSelect(discoveryModelPreset(item, snapshotSource, credential, snapshotId)); }
    catch (reason) { setError(errorMessage(reason)); }
  };

  const chooseSource = (value: string) => {
    if (value === 'new' && selectedSource) {
      const fields = newCatalogSourceFields(connectionBaseURLs, Number(definitionId), selectedSource);
      setBaseURL(fields.base_url); setGroup(fields.external_group);
      setTimeoutValue(String(fields.request_timeout_ms)); setDefinitionId(String(fields.protocol_definition_id));
    }
    setSourceChoice(value); setHistoryPage(1); setSelectedRunId(null); setHistory([]);
  };

  return <>
    <Modal open title={`模型列表 · ${displayName || formatCredentialDisplayName(credential.credential_code, credential.pool_name, 0)}`} width="max-w-4xl" onClose={() => { if (!pending) onClose(); }}>
      <div className="space-y-4">
        <p className="text-xs text-[var(--text-secondary)]">打开、搜索及查看历史只读取已保存列表。点击“拉取最新”才访问此 Key 的上游。</p>
        {error && <ErrorNotice message={error} />}
        <form onSubmit={pull} className="space-y-3 rounded-lg border border-[var(--border-soft)] p-3">
          <div className="flex flex-wrap items-end gap-2">
            <label className="min-w-0 flex-1 text-xs font-semibold">拉取配置<Select value={sourceChoice} onChange={chooseSource} disabled={loading || pending} options={[
              ...sources.map(source => ({ value: String(source.id), label: `${source.external_group} · ${source.base_url}${source.status === 'active' ? '' : '（已停用）'}` })),
              { value: 'new', label: '新建拉取配置' },
            ]} className="mt-1 w-full" /></label>
            <Button type="button" size="sm" variant="ghost" disabled={!canEdit || pending} onClick={() => setProtocolsOpen(true)}><Settings2 size={14} />协议配置</Button>
            <Button type="submit" size="sm" loading={pending} disabled={!writable || loading || (!newSource && (selectedSource?.status !== 'active' || pendingRun(history[0])))}><RefreshCw size={14} />{newSource ? '保存配置并拉取' : pendingRun(history[0]) ? '正在拉取' : '拉取最新'}</Button>
          </div>
          {newSource ? <div className="grid gap-3 sm:grid-cols-2">
            <label className="text-xs font-semibold">上游地址<input value={baseURL} onChange={event => setBaseURL(event.target.value)} list={connectionBaseURLs.length > 1 ? `catalog-origins-${credential.id}` : undefined} placeholder="https://api.example.com" className={inputClass} disabled={!writable || pending} />{connectionBaseURLs.length > 1 && <><datalist id={`catalog-origins-${credential.id}`}>{connectionBaseURLs.map(url => <option key={url} value={url} />)}</datalist><span className="mt-1 block font-normal text-[var(--text-secondary)]">已有多个上游地址，请选择本次拉取地址。</span></>}</label>
            <label className="text-xs font-semibold">模型列表协议<Select value={definitionId} onChange={setDefinitionId} options={[{ value: '', label: '请选择数据库协议' }, ...definitions.map(item => ({ value: String(item.id), label: `${item.display_name} v${item.protocol_version}` }))]} className="mt-1 w-full" disabled={!writable || pending} /></label>
            <label className="text-xs font-semibold">上游分组<input value={group} onChange={event => setGroup(event.target.value)} maxLength={128} className={inputClass} disabled={!writable || pending} /><span className="mt-1 block font-normal text-[var(--text-secondary)]">没有上游分组时保留 default。</span></label>
            <label className="text-xs font-semibold">超时（毫秒）<input type="number" min={1000} max={120000} value={timeout} onChange={event => setTimeoutValue(event.target.value)} className={inputClass} disabled={!writable || pending} /></label>
            <p className="text-xs text-[var(--text-secondary)] sm:col-span-2">保存会授权此 Key 读取模型列表；不修改执行用途、模型绑定或价格。列表路径与鉴权方式由所选数据库协议决定。</p>
          </div> : selectedSource && <p className="text-xs text-[var(--text-secondary)]">{definitions.find(item => item.id === selectedSource.protocol_definition_id)?.display_name || '协议不可用'} · 超时 {selectedSource.request_timeout_ms} 毫秒</p>}
        </form>
        <section className="space-y-2" aria-label="历史列表">
          <label className="block text-xs font-semibold">历史列表<Select value={String(selectedRunId || '')} onChange={value => setSelectedRunId(Number(value) || null)} disabled={historyLoading} options={history.length ? history.map(run => ({ value: String(run.id), label: `${formatOpsDate(run.observed_at || run.created_at)} · ${catalogRunStateLabel(run.state)}${run.item_count === null ? '' : ` · ${run.item_count} 个模型`}` })) : [{ value: '', label: historyLoading ? '正在读取历史' : '尚未拉取' }]} className="mt-1 w-full" /></label>
          {historyTotal > 20 && <Pagination page={historyPage} pageSize={20} total={historyTotal} loading={historyLoading} pageSizeOptions={[20]} onPageChange={setHistoryPage} onPageSizeChange={() => {}} />}
          {selectedRun?.state === 'failed' && <ErrorNotice message={`拉取失败：${selectedRun.last_reason_code || '未知错误'}。历史快照仍保留，未修改已接入模型。`} />}
        </section>
        {snapshotId ? <section className="space-y-2" aria-label="快照候选模型">
          <div className="flex flex-wrap items-center justify-between gap-2"><p className="text-xs text-[var(--text-secondary)]">快照 #{snapshotId} · {selectedRun?.item_count ?? 0} 个模型 · 选择后进入接入表单，不自动上架。</p><label className="flex items-center gap-2 text-xs"><Search size={14} /><span className="sr-only">搜索快照模型</span><input value={query} onChange={event => { setQuery(event.target.value); setItemPage(1); }} maxLength={128} placeholder="搜索模型名" className="rounded-md border border-[var(--border-soft)] bg-transparent px-2 py-1.5" /></label></div>
          <div className="max-h-80 overflow-auto rounded-lg border border-[var(--border-soft)]"><table className="w-full text-left text-xs"><thead className="bg-[var(--surface-muted)]"><tr>{['上游模型名', '原文描述', '价格字段', '接口提示', '接入'].map(title => <th key={title} className="px-3 py-2 font-semibold">{title}</th>)}</tr></thead><tbody>
            {items.map(item => <tr key={item.id} className="border-t border-[var(--border-soft)]"><td className="px-3 py-2 font-mono">{item.model_code}{!item.selected_group_enabled && <span className="mt-1 block font-sans text-[var(--text-secondary)]">不属当前分组</span>}</td><td className="max-w-xs px-3 py-2"><p className="line-clamp-2" title={item.description}>{item.description || '未提供'}</p></td><td className="px-3 py-2 font-mono">{catalogPriceFacts(item)}</td><td className="px-3 py-2">{item.endpoint_types.join(' / ') || '未提供'}</td><td className="px-3 py-2"><Button type="button" size="sm" variant="ghost" disabled={!writable || pending || !item.selected_group_enabled || snapshotSource?.status !== 'active'} onClick={() => select(item)}>补全接入</Button></td></tr>)}
            {items.length === 0 && <tr><td colSpan={5} className="px-3 py-6 text-center text-[var(--text-secondary)]">{itemsLoading ? '正在读取快照' : query ? '没有匹配模型' : '此次返回空列表；已有模型未被停用'}</td></tr>}
          </tbody></table></div>
          <p className="text-[11px] text-[var(--text-secondary)]">价格字段为上游原始信息，未核实币种与单位；缺失参数不是零或不支持。原文描述不会自动写入公开介绍。</p>
          <Pagination page={itemPage} pageSize={50} total={itemTotal} loading={itemsLoading} pageSizeOptions={[50]} onPageChange={setItemPage} onPageSizeChange={() => {}} />
        </section> : <p className="py-4 text-center text-xs text-[var(--text-secondary)]">{pendingRun(selectedRun) ? '正在拉取，完成后将显示保存的模型列表' : historyLoading || loading ? '正在读取配置与历史列表' : '选择已有快照，或点击拉取最新'}</p>}
      </div>
    </Modal>
    {protocolsOpen && <ProtocolDefinitionsDialog initialKind="catalog_discovery" onClose={() => { setProtocolsOpen(false); setConnectionRevision(current => current + 1); }} />}
  </>;
};
