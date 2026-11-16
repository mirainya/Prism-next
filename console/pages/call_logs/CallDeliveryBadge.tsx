import type { APICallDeliverySummary } from '../../services/callApi';

const SUCCESS = 'bg-green-100 text-green-700';
const PROCESSING = 'bg-blue-100 text-blue-700';
const FAILED = 'bg-red-100 text-red-700';
const UNAVAILABLE = 'bg-amber-100 text-amber-700';
const REFERENCE = 'bg-gray-100 text-gray-700';

export function callDeliveryStatus(summary: APICallDeliverySummary) {
  if (summary.failed) return { label: summary.failed === summary.total ? '转存失败' : '部分转存失败', tone: FAILED };
  if (summary.unavailable) return { label: '文件不可用', tone: UNAVAILABLE };
  if (summary.expired) return { label: '文件已过期', tone: UNAVAILABLE };
  if (summary.processing) return { label: summary.checking === summary.processing ? '核对中' : '转存中', tone: PROCESSING };
  if (summary.ready === summary.total) return { label: '转存成功', tone: SUCCESS };
  if (summary.reference === summary.total) return { label: '未转存', tone: REFERENCE };
  return { label: '部分转存成功', tone: UNAVAILABLE };
}

export const CallDeliveryBadge = ({ summary, onOpen }: { summary?: APICallDeliverySummary; onOpen: () => void }) => {
  if (!summary) return <span className="text-xs text-[var(--text-secondary)]">—</span>;
  if (!summary.total) return <span className="text-xs text-[var(--text-secondary)]">暂无文件</span>;
  const status = callDeliveryStatus(summary);
  const counts = [
    summary.ready && `${summary.ready} 个成功`,
    summary.processing && `${summary.processing} 个处理中`,
    summary.failed && `${summary.failed} 个失败`,
    summary.unavailable && `${summary.unavailable} 个不可用`,
    summary.expired && `${summary.expired} 个已过期`,
    summary.reference && `${summary.reference} 个未转存`,
  ].filter(Boolean).join('，');
  return <button type="button" title={counts} aria-label={`查看转存结果：${status.label}`} onClick={event => {
    event.stopPropagation();
    onOpen();
  }} onKeyDown={event => event.stopPropagation()} className="whitespace-nowrap text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--primary)]">
    <span className={`inline-flex rounded-md px-2 py-1 text-xs font-semibold ${status.tone}`}>{status.label}</span>
    {summary.total > 1 && <span className="mt-1 block text-xs text-[var(--text-secondary)]">已转存 {summary.ready}/{summary.total}</span>}
  </button>;
};
