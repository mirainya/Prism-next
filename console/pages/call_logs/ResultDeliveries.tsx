import React, { useEffect, useState } from 'react';
import { AlertTriangle, Check, Copy, ExternalLink, FileImage, LoaderCircle, RefreshCw } from 'lucide-react';
import { fetchCallDeliveryURL, fetchCallDetail, retryCallDelivery, type APICallDeliveryURL, type APICallResultDelivery } from '../../services/callApi';

export function deliveryStatus(delivery: APICallResultDelivery) {
  if (delivery.state === 'delivery_failed' && delivery.retry_status === 'dispatching') return { label: '重新转存中', tone: 'bg-blue-100 text-blue-700' };
  if (delivery.state === 'delivery_failed' && delivery.retry_status === 'pending') return { label: '等待重新转存', tone: 'bg-blue-100 text-blue-700' };
  if (delivery.state === 'delivery_failed') return { label: '转存失败', tone: 'bg-red-100 text-red-700' };
  if (delivery.state === 'expired') return { label: '已过期', tone: 'bg-amber-100 text-amber-700' };
  if (delivery.delivery_mode === 'reference') return { label: '未转存', tone: 'bg-gray-100 text-gray-700' };
  if (delivery.available) return { label: '转存成功', tone: 'bg-green-100 text-green-700' };
  if (delivery.state === 'pending') return { label: '转存中', tone: 'bg-blue-100 text-blue-700' };
  return { label: '文件不可用', tone: 'bg-amber-100 text-amber-700' };
}

const formatBytes = (value: number) => value >= 1024 * 1024
  ? `${(value / (1024 * 1024)).toFixed(2)} MB`
  : value > 0 ? `${(value / 1024).toFixed(1)} KB` : '-';

export const CallDeliverySummary: React.FC<{ deliveries: APICallResultDelivery[]; onOpen: () => void }> = ({ deliveries, onOpen }) => {
  if (deliveries.length === 0) return null;
  const failed = deliveries.filter(item => item.state === 'delivery_failed').length;
  const unavailable = deliveries.filter(item => item.delivery_mode === 'managed_copy' && item.state === 'ready' && !item.available).length;
  return <section className={`flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4 ${failed || unavailable ? 'border-amber-200 bg-amber-50' : 'border-[var(--border-soft)]'}`}>
    <div>
      <div className="text-sm font-bold text-[var(--text-primary)]">转存结果</div>
      <p className="mt-1 text-sm text-[var(--text-secondary)]">共 {deliveries.length} 个结果文件{failed > 0 && `，${failed} 个转存失败`}{unavailable > 0 && `，${unavailable} 个文件不可用`}</p>
    </div>
    <button onClick={onOpen} className="text-sm font-semibold text-[var(--primary)]">查看文件与地址</button>
  </section>;
};

const ResultDeliveryCard: React.FC<{ callId: string; delivery: APICallResultDelivery; onRefresh: () => Promise<void> }> = ({ callId, delivery, onRefresh }) => {
  const [link, setLink] = useState<APICallDeliveryURL | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);
  const [revision, setRevision] = useState(0);
  const [retrying, setRetrying] = useState(false);
  const retry = async () => {
    setRetrying(true);
    setError('');
    try {
      await retryCallDelivery(callId, delivery.id);
      await onRefresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '重新转存失败');
    } finally {
      setRetrying(false);
    }
  };
  useEffect(() => {
    setLink(null);
    setCopied(false);
    setError('');
    if (!delivery.available) return;
    const controller = new AbortController();
    setLoading(true);
    fetchCallDeliveryURL(callId, delivery.id, controller.signal)
      .then(value => { if (!controller.signal.aborted) setLink(value); })
      .catch(reason => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '获取文件地址失败'); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [callId, delivery.id, delivery.available, delivery.updated_at, revision]);
  const status = deliveryStatus(delivery);
  const copyURL = async () => {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.url);
      setCopied(true);
      setError('');
    } catch {
      setError('复制失败，请选中下方地址手动复制');
    }
  };
  const fileLabel = delivery.content_type.startsWith('image/') ? '图片' : delivery.content_type.startsWith('video/') ? '视频' : '文件';
  return <section className="rounded-lg border border-[var(--border-soft)] p-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-2 text-sm font-bold text-[var(--text-primary)]"><FileImage size={17} />{fileLabel} {delivery.result_ordinal + 1}<span className={`rounded-md px-2 py-1 text-xs ${status.tone}`}>{status.label}</span></div>
      <span className="text-xs text-[var(--text-secondary)]">{delivery.content_type || '-'} · {formatBytes(delivery.content_length)}</span>
    </div>
    {delivery.filename && <p className="mt-3 break-all font-mono text-xs text-[var(--text-primary)]">{delivery.filename}</p>}
    {delivery.storage_path && <div className="mt-3"><div className="text-xs text-[var(--text-secondary)]">保存路径</div><p className="mt-1 break-all font-mono text-xs text-[var(--text-primary)]">{delivery.storage_path}</p></div>}
    {delivery.reason_message && <div className="mt-3 flex items-start gap-2 text-sm text-red-700"><AlertTriangle size={16} className="mt-0.5 shrink-0" /><div>{delivery.reason_message}{delivery.reason_code && <div className="mt-1 font-mono text-xs">{delivery.reason_code}</div>}</div></div>}
    {delivery.failure_http_status && <p className="mt-2 text-xs text-[var(--text-secondary)]">HTTP {delivery.failure_http_status}</p>}
    {delivery.state === 'delivery_failed' && <div className="mt-3 space-y-2 text-xs text-[var(--text-secondary)]">
      {delivery.retry_status === 'dispatching' ? <p role="status">正在重新转存原结果</p> : delivery.retry_status === 'pending' ? <p role="status">已排队{delivery.retry_at && `，下次尝试：${new Date(delivery.retry_at).toLocaleTimeString('zh-CN', { hour12: false })}`}</p> : delivery.retry_status === 'dead_letter' ? <p>自动重试已停止，请查看失败原因。</p> : null}
      {!!delivery.retry_attempts && <p>本轮已尝试 {delivery.retry_attempts} 次</p>}
      {delivery.can_retry && <button onClick={retry} disabled={retrying} className="inline-flex items-center gap-1 font-semibold text-[var(--primary)] disabled:opacity-50"><RefreshCw size={13} className={retrying ? 'animate-spin' : ''} />{retrying ? '正在提交' : '重新转存'}</button>}
      {(delivery.can_retry || delivery.retry_status === 'pending' || delivery.retry_status === 'dispatching') && <p>使用原生成结果，不重新生图或扣费。</p>}
    </div>}
    {delivery.delivery_mode === 'reference' && <p className="mt-3 text-sm text-[var(--text-secondary)]">该结果使用上游链接，没有转存文件。</p>}
    {loading && <p role="status" className="mt-3 flex items-center gap-2 text-sm text-[var(--text-secondary)]"><LoaderCircle size={15} className="animate-spin" />正在获取文件地址</p>}
    {link && <div className="mt-4">
      <label className="text-xs text-[var(--text-secondary)]" htmlFor={`delivery-url-${delivery.id}`}>文件地址</label>
      <textarea id={`delivery-url-${delivery.id}`} aria-label="文件地址" readOnly value={link.url} rows={3} className="mt-1 w-full resize-none rounded-md border border-[var(--border-soft)] bg-[var(--surface-muted)] p-2 font-mono text-xs text-[var(--text-primary)]" />
      <div className="mt-2 flex flex-wrap items-center gap-4 text-xs">
        <a href={link.url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer" className="inline-flex items-center gap-1 text-[var(--primary)]"><ExternalLink size={14} />打开原文件</a>
        <button onClick={copyURL} className="inline-flex items-center gap-1 text-[var(--primary)]">{copied ? <Check size={14} /> : <Copy size={14} />}{copied ? '已复制' : '复制地址'}</button>
        <span className="text-[var(--text-secondary)]">地址有效至 {new Date(link.expires_at).toLocaleString('zh-CN', { hour12: false })}</span>
      </div>
    </div>}
    {error && <p role="alert" className="mt-3 break-words text-sm text-red-700">{error}</p>}
    {delivery.available && <button onClick={() => setRevision(value => value + 1)} disabled={loading} className="mt-3 inline-flex items-center gap-1 text-xs text-[var(--primary)] disabled:opacity-50"><RefreshCw size={13} />刷新文件地址</button>}
    {delivery.retention_until && <p className="mt-3 text-xs text-[var(--text-secondary)]">文件保留至 {new Date(delivery.retention_until).toLocaleString('zh-CN', { hour12: false })}</p>}
  </section>;
};

export const ResultDeliveries: React.FC<{ callId: string; deliveries: APICallResultDelivery[] }> = ({ callId, deliveries }) => {
  const [items, setItems] = useState(deliveries);
  const [refreshError, setRefreshError] = useState('');
  useEffect(() => { setItems(deliveries); }, [deliveries]);
  const refresh = async () => {
    try {
      const detail = await fetchCallDetail(callId);
      setItems(detail.deliveries || []);
      setRefreshError('');
    } catch (reason) {
      setRefreshError(reason instanceof Error ? reason.message : '刷新转存状态失败');
      throw reason;
    }
  };
  const pending = items.some(item => item.state === 'delivery_failed' && (item.retry_status === 'pending' || item.retry_status === 'dispatching'));
  useEffect(() => {
    if (!pending) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const detail = await fetchCallDetail(callId);
        if (active) { setItems(detail.deliveries || []); setRefreshError(''); }
      } catch {
        if (active) setRefreshError('刷新转存状态失败');
      }
      if (active) timer = setTimeout(poll, 3000);
    };
    timer = setTimeout(poll, 3000);
    return () => { active = false; clearTimeout(timer); };
  }, [callId, pending]);
  if (items.length === 0) return <div className="py-16 text-center text-sm text-[var(--text-secondary)]">暂无结果文件记录</div>;
  return <div className="space-y-4"><div className="flex items-center justify-between gap-2"><button onClick={() => { void refresh().catch(() => {}); }} className="inline-flex items-center gap-1 text-xs text-[var(--primary)]"><RefreshCw size={13} />刷新转存状态</button>{refreshError && <span role="alert" className="text-xs text-red-700">{refreshError}</span>}</div>{items.map(delivery => <ResultDeliveryCard key={delivery.id} callId={callId} delivery={delivery} onRefresh={refresh} />)}</div>;
};
