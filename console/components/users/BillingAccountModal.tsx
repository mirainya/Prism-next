import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Modal } from '../ui';
import { fetchUserBillingAccount, unfreezeUserBillingAccount } from '../../services/userApi';
import { BillingAccountDetail, User } from '../../types';

const accountLabels = { open: '正常', frozen: '已冻结', closed: '已关闭' };
export const formatBillingAmount = (amount: string) => amount.includes('.') ? amount.replace(/0+$/, '').replace(/\.$/, '') : amount;
const formatDate = (value: string | null) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '未记录';
const reasonLabel = (reason: string) => ({
  settlement_overrun: '结算异常（预扣超额、授信或 Key 预算超限）',
  manual_overrun_recovery: '管理员手动解冻',
  account_opened: '账户创建',
  legacy_balance_import: '历史账户导入',
}[reason] || '其他原因，需要人工核查');

interface Props {
  user: User;
  onClose: () => void;
  onUpdated: () => void;
}

export const BillingAccountModal: React.FC<Props> = ({ user, onClose, onUpdated }) => {
  const [detail, setDetail] = useState<BillingAccountDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setDetail(null);
    setError('');
    fetchUserBillingAccount(user.id, controller.signal)
      .then(data => { if (!controller.signal.aborted) setDetail(data); })
      .catch(err => { if (!controller.signal.aborted) setError(err.message || '读取计费账户失败'); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [user.id, refresh]);

  const handleUnfreeze = async () => {
    if (!detail?.can_unfreeze || !note.trim() || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      await unfreezeUserBillingAccount(user.id, detail.account.state_event_id, note);
      onUpdated();
      onClose();
    } catch (err: any) {
      setError(err.message || '解冻失败，请重新查看账户状态');
      // Another freeze must be reviewed before issuing another recovery request.
      setDetail(null);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal open onClose={() => { if (!submitting) onClose(); }} title={`计费账户 · ${user.username}`} width="max-w-3xl">
      <div className="modal-form">
        <div className="modal-scroll-body space-y-4">
          {loading && <p className="text-sm text-[var(--text-secondary)]" role="status">正在读取计费账户…</p>}
          {error && <div role="alert" className="rounded-lg bg-red-50 p-3 text-sm text-red-700">{error}</div>}
          {!loading && !detail && <button type="button" className="modal-button modal-button-secondary" onClick={() => setRefresh(value => value + 1)}>重新读取</button>}
          {detail && <>
            <div className="rounded-xl border border-[var(--border-soft)] p-4 space-y-2 text-sm">
              <div className="flex justify-between gap-3"><span>账户 #{detail.account.id}</span><strong>{accountLabels[detail.account.status]}</strong></div>
              <p className="text-[var(--text-secondary)]">{reasonLabel(detail.account.state_reason)}</p>
              <p className="text-[var(--text-secondary)]">状态变更：{formatDate(detail.account.state_changed_at)}</p>
              <dl className="grid grid-cols-2 gap-3 pt-2 sm:grid-cols-4">
                {[
                  ['账面余额', detail.account.posted_balance], ['授信额度', detail.account.credit_limit],
                  ['预留金额', detail.account.held_amount], ['可用资金', detail.account.available],
                ].map(([label, amount]) => <div key={label}><dt className="text-xs text-[var(--text-secondary)]">{label}</dt><dd className="mt-1 break-all font-mono">{formatBillingAmount(amount)}</dd></div>)}
              </dl>
              <p className="text-xs text-[var(--text-secondary)]">单位：{detail.account.currency}。可用资金 = 账面余额 + 授信额度 − 预留金额。</p>
            </div>

            <div>
              <h4 className="mb-2 text-sm font-bold">最近超额或欠款结算</h4>
              <p className="mb-2 text-xs text-[var(--text-secondary)]">展示最近 10 笔，供核查；历史记录不一定对应本次冻结。</p>
              {detail.recent_anomalies.length === 0 ? <p className="text-sm text-[var(--text-secondary)]">暂无记录</p> : (
                <div className="overflow-x-auto rounded-lg border border-[var(--border-soft)]">
                  <table className="w-full min-w-[600px] text-left text-xs">
                    <thead><tr className="bg-[var(--surface)]">
                      {['调用 / Key', '预扣', '实际扣费', '超出预扣', '预算超额', '结算后欠款'].map(label => <th key={label} className="p-2 whitespace-nowrap">{label}</th>)}
                    </tr></thead>
                    <tbody>{detail.recent_anomalies.map(row => <tr key={row.call_id} className="border-t border-[var(--border-soft)]">
                      <td className="p-2"><Link onClick={onClose} to={`/calls?call_id=${encodeURIComponent(row.public_call_id)}&user_id=${user.id}&token_id=${row.token_id}`} className="block max-w-48 break-all font-mono text-[var(--primary)]" title={row.public_call_id}>{row.public_call_id}</Link><div className="mt-1 text-[var(--text-secondary)]"><Link onClick={onClose} to={`/tokens?user_id=${user.id}&token_id=${row.token_id}`} className="text-[var(--primary)]">Key #{row.token_id}</Link> · {formatDate(row.created_at)}</div></td>
                      {[row.authorized_amount, row.actual_amount, row.authorization_excess, row.budget_excess, row.debt_after].map((amount, i) => <td key={i} className="p-2 font-mono whitespace-nowrap">{formatBillingAmount(amount)}</td>)}
                    </tr>)}</tbody>
                  </table>
                </div>
              )}
            </div>

            {detail.budget_issues.length > 0 && <div className="rounded-lg bg-amber-50 p-3 text-sm text-amber-900">
              <p className="font-bold">当前预算仍超限</p>
              {detail.budget_issues.map(issue => <p key={issue.token_id}><Link onClick={onClose} to={`/tokens?user_id=${user.id}&token_id=${issue.token_id}`} className="underline">Key #{issue.token_id}</Link>：限额 {formatBillingAmount(issue.limit)}，已用 {formatBillingAmount(issue.used)}，预留 {formatBillingAmount(issue.held)}</p>)}
            </div>}

            {detail.account.status === 'frozen' && <>
              {!detail.can_unfreeze && <p role="status" className="text-sm text-red-700">{detail.unfreeze_block_reason}</p>}
              <label className="block text-sm font-bold" htmlFor="billing-unfreeze-note">解冻处理备注</label>
              <textarea id="billing-unfreeze-note" value={note} onChange={event => setNote(event.target.value)} maxLength={500} rows={3}
                disabled={!detail.can_unfreeze || submitting} placeholder="填写核查结果与解冻原因"
                className="w-full rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-3 text-sm disabled:opacity-50" />
              <p className="text-xs text-[var(--text-secondary)]">确认后恢复该账户的新调用。余额和账单保持不变；新的结算异常仍可能再次触发冻结。</p>
            </>}
          </>}
        </div>
        <div className="modal-footer">
          <button type="button" className="modal-button modal-button-secondary" disabled={submitting} onClick={onClose}>关闭</button>
          {detail?.account.status === 'frozen' && <button type="button" className="modal-button modal-button-primary"
            disabled={!detail.can_unfreeze || !note.trim() || submitting} onClick={handleUnfreeze}>
            {submitting ? '正在解冻…' : '确认解冻'}
          </button>}
        </div>
      </div>
    </Modal>
  );
};
