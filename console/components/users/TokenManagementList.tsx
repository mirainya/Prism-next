import React, { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Modal, Pagination, Select } from '../ui';
import { ApiToken, UserRole } from '../../types';
import { changeTokenAccess, fetchManagedTokens, fetchTokenAccessHistory, ManagedToken, TokenAccessEvent, TokenRateSettings } from '../../services/tokenManagementApi';
import { formatFundsAmount, formatFundsDisplay } from './FundsAdjustmentModal';
import { TokenPolicyModal } from './TokenPolicyModal';

const labels = { active: '使用中', paused: '已停用', expired: '已过期', revoked: '已作废' };
const actionLabels = { pause: '停用', resume: '启用', revoke: '作废' };
const inputClass = 'rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-2 text-sm';
const buttonClass = 'rounded-md px-2 py-1 text-xs font-semibold text-[var(--primary)] hover:bg-[var(--primary-lighter)] disabled:opacity-40';
const date = (value: string | null) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '长期有效';
const reason = (code: string) => ({ owner_pause: '用户停用', admin_pause: '管理员停用', owner_resume: '用户启用', admin_resume: '管理员启用', owner_revoke: '用户作废', admin_revoke: '管理员作废', owner_revoked: '用户作废', owner_policy_updated: '用户修改调用设置', admin_policy_updated: '管理员修改调用设置' }[code] || '使用设置变更');

interface Props {
  revision: number;
  rateSettings: TokenRateSettings | null;
  currency?: string;
  onBudget: (token: ApiToken) => void;
  onEdit: (token: ApiToken) => void;
  onStorage: (token: ApiToken) => void;
}

export const TokenManagementList: React.FC<Props> = ({ revision, rateSettings, currency, onBudget, onEdit, onStorage }) => {
  const [params, setParams] = useSearchParams();
  const [actor] = useState(() => { try { return JSON.parse(localStorage.getItem('prism_user') || '{}'); } catch { return {}; } });
  const isAdmin = actor.role === UserRole.ADMIN;
  const ownUser = String(actor.id || '');
  const urlUser = params.get('user_id') || '';
  const urlToken = params.get('token_id') || '';
  const [draft, setDraft] = useState({ user_id: urlUser || ownUser, token_id: urlToken, search: '', state: urlToken ? '' : 'active' });
  const [filters, setFilters] = useState(draft);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [rows, setRows] = useState<ManagedToken[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [operation, setOperation] = useState<{ token: ManagedToken; action: 'pause' | 'resume' | 'revoke' } | null>(null);
  const [note, setNote] = useState('');
  const [saving, setSaving] = useState(false);
  const [operationError, setOperationError] = useState('');
  const [history, setHistory] = useState<ManagedToken | null>(null);
  const [historyRows, setHistoryRows] = useState<TokenAccessEvent[]>([]);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const [historyRefresh, setHistoryRefresh] = useState(0);
  const [policyTarget, setPolicyTarget] = useState<ManagedToken | null>(null);
  const [management, setManagement] = useState<ManagedToken | null>(null);

  useEffect(() => {
    const next = { user_id: urlUser || ownUser, token_id: urlToken, search: '', state: urlToken ? '' : 'active' };
    setDraft(next); setFilters(next); setPage(1);
    setManagement(null);
  }, [urlUser, urlToken, ownUser]);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError(''); setRows([]);
    fetchManagedTokens({ ...filters, page, page_size: pageSize }, controller.signal).then(result => {
      if (controller.signal.aborted) return;
      setRows(result.items); setTotal(result.total);
      const lastPage = Math.max(1, Math.ceil(result.total / pageSize));
      if (page > lastPage) setPage(lastPage);
    }).catch(err => { if (!controller.signal.aborted) setError(err.message || '读取 Key 失败'); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [filters, page, pageSize, revision, refresh]);

  useEffect(() => {
    if (!history) return;
    const controller = new AbortController();
    setHistoryLoading(true); setHistoryError(''); setHistoryRows([]);
    fetchTokenAccessHistory(history.id, historyPage, controller.signal).then(result => {
      if (!controller.signal.aborted) { setHistoryRows(result.items); setHistoryTotal(result.total); }
    }).catch(err => { if (!controller.signal.aborted) setHistoryError(err.message || '读取历史失败'); })
      .finally(() => { if (!controller.signal.aborted) setHistoryLoading(false); });
    return () => controller.abort();
  }, [history, historyPage, historyRefresh]);

  const openOperation = (token: ManagedToken, action: 'pause' | 'resume' | 'revoke') => {
    setManagement(null); setNote(''); setOperationError(''); setOperation({ token, action });
  };
  const applyOperation = async () => {
    if (!operation || !note.trim() || saving) return;
    setSaving(true); setOperationError('');
    try {
      await changeTokenAccess(operation.token, operation.action, note.trim());
      setOperation(null); setRefresh(value => value + 1);
    } catch (err: any) {
      setOperationError(err.message || '操作失败，请刷新后重试');
      setRefresh(value => value + 1);
    } finally { setSaving(false); }
  };
  const ownerToken = (token: ManagedToken): ApiToken => ({ ...token, status: 'active' });
  const queryFor = (token: ManagedToken) => new URLSearchParams({ user_id: token.userID, token_id: token.id }).toString();
  const selectFilter = (field: 'user_id' | 'state', value: string) => {
    const next = { ...draft, [field]: value };
    setDraft(next); setFilters(next); setPage(1);
  };
  const editable = management?.userID === ownUser && management?.status === 'active' && management?.ownerEnabled;
  const manageAction = (action: (token: ApiToken) => void) => {
    if (!management) return;
    action(ownerToken(management)); setManagement(null);
  };

  return <section className="overflow-hidden rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)]">
    <form className="flex flex-wrap items-center gap-2 p-4" onSubmit={event => { event.preventDefault(); setPage(1); setFilters({ ...draft }); }}>
      <input aria-label="搜索名称" placeholder={isAdmin && !draft.user_id ? '搜索 Key 或用户名' : '搜索 Key 名称'} value={draft.search} maxLength={100} className={`${inputClass} min-w-48 flex-1`} onChange={event => setDraft({ ...draft, search: event.target.value })} />
      {isAdmin && <Select value={draft.user_id} onChange={value => selectFilter('user_id', value)} options={[{ value: ownUser, label: '我的 Key' }, { value: '', label: '全部用户' }, ...(draft.user_id && draft.user_id !== ownUser ? [{ value: draft.user_id, label: `用户 #${draft.user_id}` }] : [])]} />}
      <Select value={draft.state} onChange={value => selectFilter('state', value)} options={[{ value: '', label: '全部状态' }, ...Object.entries(labels).map(([value, label]) => ({ value, label }))]} />
      <button className="modal-button modal-button-primary" type="submit">查询</button>
      <button className="modal-button modal-button-secondary" type="button" onClick={() => setRefresh(value => value + 1)}>刷新</button>
    </form>
    {filters.token_id && <p className="px-4 pb-3 text-xs text-[var(--text-secondary)]">正在查看 Key #{filters.token_id}<button type="button" className={buttonClass} onClick={() => setParams({})}>返回我的 Key</button></p>}
    {error && <p role="alert" className="px-4 pb-3 text-sm text-red-700">{error}</p>}
    <div className="overflow-x-auto">
      <table aria-label="API Key 列表" className="w-full min-w-[640px] text-left text-sm">
        <thead className="bg-[var(--surface-muted)]"><tr>{['名称', '状态', `剩余消费额度${currency ? `（${currency}）` : ''}`, '结果保存', '操作'].map(label => <th className="p-4 whitespace-nowrap" key={label}>{label}</th>)}</tr></thead>
        <tbody>{rows.map(token => <tr key={token.id} className="border-t border-[var(--border-soft)]">
          <td className="p-4"><strong>{token.name}</strong><p className="mt-1 text-xs text-[var(--text-secondary)]">#{token.id} · {token.key}</p>{filters.user_id !== ownUser && <p className="mt-1 text-xs text-[var(--text-secondary)]">{token.username || '用户已删除'}</p>}</td>
          <td className="p-4"><span className={`inline-block rounded-full px-2 py-1 text-xs font-semibold ${token.status === 'active' && token.ownerEnabled ? 'bg-emerald-50 text-emerald-700' : 'bg-[var(--surface-muted)] text-[var(--text-secondary)]'}`}>{token.ownerEnabled ? labels[token.status] : '所属用户已停用'}</span><p className="mt-1 text-xs text-[var(--text-secondary)]">{token.expiresAt ? `${new Date(token.expiresAt).toLocaleDateString('zh-CN')} 到期` : '长期有效'}</p></td>
          <td className="p-4 font-semibold tabular-nums" title={token.budget?.remaining == null ? undefined : formatFundsAmount(token.budget.remaining)}>{token.budget ? token.budget.remaining === null ? '不限额' : formatFundsDisplay(token.budget.remaining) : '未设置'}</td>
          <td className="p-4 text-xs text-[var(--text-secondary)]">{token.xfsStorage.configured ? '已开启转存' : '返回原始链接'}</td>
          <td className="p-4"><button type="button" className={buttonClass} aria-label={`管理 ${token.name}`} onClick={() => setManagement(token)}>管理</button></td>
        </tr>)}</tbody>
      </table>
      {loading && <p role="status" className="p-4 text-sm">正在读取 Key…</p>}
      {!loading && !error && rows.length === 0 && <p className="p-4 text-sm">{filters.state === 'active' ? '没有使用中的 API Key' : '没有符合条件的 API Key'}</p>}
    </div>
    <Pagination page={page} pageSize={pageSize} total={total} loading={loading} onPageChange={setPage} onPageSizeChange={size => { setPageSize(size); setPage(1); }} />
    <p className="border-t border-[var(--border-soft)] p-3 text-xs text-[var(--text-secondary)]">停用、过期和作废的 Key 可通过状态筛选查看。</p>
    <Modal open={management !== null} title={`管理 API Key · ${management?.name || ''}`} onClose={() => setManagement(null)} width="max-w-xl">
      {management && <div className="modal-form"><div className="modal-scroll-body space-y-5">
        <div><p className="text-sm font-semibold">{labels[management.status]} · {management.expiresAt ? `${date(management.expiresAt)} 到期` : '长期有效'}</p><p className="mt-1 text-xs text-[var(--text-secondary)]">#{management.id} · {management.key} · 所属用户：{management.username || `#${management.userID}`}</p>{!management.ownerEnabled && <p className="mt-2 text-xs text-amber-700">所属用户已停用或删除</p>}</div>
        <section className="rounded-lg bg-[var(--surface-muted)] p-4">
          <h4 className="text-sm font-semibold">消费额度{currency ? `（${currency}）` : ''}</h4>
          {management.budget ? <dl className="mt-3 grid grid-cols-2 gap-3 text-xs">{[
            ['消费上限', management.budget.limit === null ? '不限额' : formatFundsAmount(management.budget.limit)],
            ['剩余额度', management.budget.remaining === null ? '不限额' : formatFundsAmount(management.budget.remaining)],
            ['已消费', formatFundsAmount(management.budget.used)], ['处理中占用', formatFundsAmount(management.budget.held)],
          ].map(([label, value]) => <div key={label}><dt className="text-[var(--text-secondary)]">{label}</dt><dd className="mt-1 break-all font-semibold tabular-nums">{value}</dd></div>)}</dl> : <p className="mt-2 text-xs">尚未设置消费额度</p>}
        </section>
        <section><h4 className="text-sm font-semibold">设置</h4><div className="mt-2 flex flex-wrap gap-2">
          {editable && <><button type="button" className={buttonClass} onClick={() => manageAction(onEdit)}>修改名称</button><button type="button" className={buttonClass} onClick={() => manageAction(onBudget)}>增加额度</button><button type="button" className={buttonClass} onClick={() => manageAction(onStorage)}>文件存储</button></>}
          {management.status !== 'revoked' && <button type="button" className={buttonClass} onClick={() => { setPolicyTarget(management); setManagement(null); }}>调用设置</button>}
        </div><p className="mt-2 text-xs text-[var(--text-secondary)]">{rateSettings ? rateSettings.enabled ? `每分钟最多调用 ${Math.min(management.rateLimit > 0 ? management.rateLimit : rateSettings.requests_per_min, rateSettings.requests_per_min)} 次` : '当前未启用调用频率限制' : '调用频率设置尚未读取'}</p></section>
        <section className="border-t border-[var(--border-soft)] pt-4"><h4 className="text-sm font-semibold">使用状态</h4><div className="mt-2 flex flex-wrap gap-2">
          {management.status === 'active' && <button type="button" className={buttonClass} onClick={() => openOperation(management, 'pause')}>停用</button>}
          {management.status === 'paused' && <button type="button" disabled={!management.ownerEnabled} className={buttonClass} onClick={() => openOperation(management, 'resume')}>启用</button>}
          {management.status !== 'revoked' && <button type="button" className={`${buttonClass} !text-red-600`} onClick={() => openOperation(management, 'revoke')}>作废</button>}
        </div></section>
        <div className="flex flex-wrap gap-3 border-t border-[var(--border-soft)] pt-4 text-xs text-[var(--primary)]"><button type="button" onClick={() => { setHistoryPage(1); setHistory(management); setManagement(null); }}>变更记录</button><Link to={`/calls?${queryFor(management)}`} onClick={() => setManagement(null)}>查看调用记录</Link></div>
      </div><div className="modal-footer"><button type="button" className="modal-button modal-button-secondary" onClick={() => setManagement(null)}>关闭</button></div></div>}
    </Modal>
    {policyTarget && <TokenPolicyModal key={policyTarget.id} token={policyTarget} settings={rateSettings} onUpdated={() => setRefresh(value => value + 1)} onClose={() => setPolicyTarget(null)} />}

    <Modal open={operation !== null} title={`${operation ? actionLabels[operation.action] : ''} Key`} onClose={() => { if (!saving) setOperation(null); }} width="max-w-lg">
      <div className="modal-form"><div className="modal-scroll-body space-y-3">
        <p className="text-sm">{operation?.token.name} · #{operation?.token.id} · 用户 #{operation?.token.userID}</p>
        <p className="text-sm">{operation?.action === 'revoke' ? '作废后不能再启用，历史调用和账单保留。' : operation?.action === 'pause' ? '停用后不能发起新调用，之后可重新启用。已提交的任务继续处理。' : '重新允许此 Key 发起调用，账户资金和消费上限不变。'}</p>
        <label htmlFor="token-access-note" className="block text-sm font-semibold">操作备注</label>
        <textarea id="token-access-note" rows={3} maxLength={500} value={note} disabled={saving} onChange={event => setNote(event.target.value)} className={`${inputClass} w-full`} />
        {operationError && <p role="alert" className="text-sm text-red-700">{operationError}</p>}
      </div><div className="modal-footer"><button type="button" disabled={saving} className="modal-button modal-button-secondary" onClick={() => setOperation(null)}>关闭</button><button type="button" disabled={saving || !note.trim() || !!operationError} className="modal-button modal-button-primary" onClick={applyOperation}>{saving ? '正在处理…' : `确认${operation ? actionLabels[operation.action] : ''}`}</button></div></div>
    </Modal>
    <Modal open={history !== null} title={`变更记录 · ${history?.name || ''}`} onClose={() => setHistory(null)} width="max-w-2xl">
      <div className="modal-form"><div className="modal-scroll-body space-y-3">
        <p className="text-xs text-[var(--text-secondary)]">记录启用、停用、作废和调用设置的变更。</p>
        {historyLoading && <p role="status">正在读取历史…</p>}
        {historyError && <p role="alert" className="text-red-700">{historyError}<button type="button" className={buttonClass} onClick={() => setHistoryRefresh(value => value + 1)}>重试</button></p>}
        {historyRows.map(event => <div key={event.id} className="rounded-lg border border-[var(--border-soft)] p-3 text-sm"><strong>{reason(event.reason_code)}</strong><p className="mt-1 text-xs">{event.actor_user_id === Number(actor.id) ? actor.username || '当前用户' : `用户 #${event.actor_user_id}`} · {date(event.created_at)}</p>{event.note && <p className="mt-2 whitespace-pre-wrap break-words text-sm text-[var(--text-secondary)]">{event.note}</p>}</div>)}
        {!historyLoading && !historyError && !historyRows.length && <p>暂无变更记录</p>}
      </div><Pagination page={historyPage} pageSize={10} total={historyTotal} loading={historyLoading} onPageChange={setHistoryPage} onPageSizeChange={() => {}} pageSizeOptions={[10]} /><div className="modal-footer"><button type="button" className="modal-button modal-button-secondary" onClick={() => setHistory(null)}>关闭</button></div></div>
    </Modal>
  </section>;
};
