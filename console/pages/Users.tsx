import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {Users as UsersIcon, Search, Calendar, Shield, Plus} from 'lucide-react';
import { useAppDialog } from '../components/ui';
import {fetchUsers, updateUserRole, updateUserStatus, rechargeUser} from '../services/api';
import { User, UserRole } from '../types';
import { BillingAccountModal } from '../components/users/BillingAccountModal';
import { FundsAdjustmentModal, formatFundsAmount } from '../components/users/FundsAdjustmentModal';

const Users: React.FC = () => {
  const { askConfirmation, showAlert } = useAppDialog();
  const [users, setUsers] = useState<User[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [updatingUserID, setUpdatingUserID] = useState<string | null>(null);
  const [billingTarget, setBillingTarget] = useState<User | null>(null);
    const [rechargeTarget, setRechargeTarget] = useState<User | null>(null);

  const loadUsers = () => {
    setIsLoading(true);
    setLoadError('');
    return fetchUsers()
      .then(data => setUsers(data))
      .catch(err => setLoadError(err.message || '读取用户失败，请重试。'))
      .finally(() => setIsLoading(false));
  };

  useEffect(() => {
    loadUsers();
  }, []);

  const handleRoleChange = async (user: User) => {
    if (isLastAdministrator(user) || updatingUserID) return;
    const newRole = user.role === UserRole.ADMIN ? UserRole.USER : UserRole.ADMIN;
    const roleLabel = newRole === UserRole.ADMIN ? '管理员' : '普通用户';
    const confirmed = await askConfirmation({
      title: '修改用户角色？',
      description: `将“${user.username}”的角色修改为${roleLabel}。`,
      confirmLabel: '修改角色',
      tone: 'warning',
    });
    if (!confirmed) return;
    setUpdatingUserID(user.id);
    try {
      await updateUserRole(user.id, newRole);
      await loadUsers();
    } catch (err: any) {
      await showAlert({ title: '操作失败', description: err.message || '角色修改失败，请稍后重试。', tone: 'danger' });
    } finally {
      setUpdatingUserID(null);
    }
  };

  const handleStatusChange = async (user: User) => {
    if (isLastAdministrator(user) || updatingUserID) return;
    const nextStatus = user.status === 1 ? 0 : 1;
    const confirmed = await askConfirmation({
      title: nextStatus === 0 ? '停用用户？' : '恢复用户？',
      description: nextStatus === 0
        ? `停用“${user.username}”后，该用户不能登录，其全部 Key 不能发起新调用。已提交任务继续处理。`
        : `恢复“${user.username}”登录及其仍有效的 Key 调用权限。`,
      confirmLabel: nextStatus === 0 ? '确认停用' : '确认恢复',
      tone: 'warning',
    });
    if (!confirmed) return;
    setUpdatingUserID(user.id);
    try {
      await updateUserStatus(user.id, nextStatus);
      await loadUsers();
    } catch (err: any) {
      await showAlert({ title: '操作失败', description: err.message || '用户状态修改失败，请重试。', tone: 'danger' });
    } finally {
      setUpdatingUserID(null);
    }
  };

  const enabledAdministrators = users.filter(user => user.role === UserRole.ADMIN && user.status === 1).length;
  const isLastAdministrator = (user: User) => user.role === UserRole.ADMIN && user.status === 1 && enabledAdministrators === 1;
  const query = search.trim().toLowerCase();
  const visibleUsers = users.filter(user =>
    (!query || user.username.toLowerCase().includes(query) || user.id.includes(query)) &&
    (statusFilter === '' || String(user.status) === statusFilter));

  return (
    <div className="space-y-4 md:space-y-6">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div>
          <h1 className="text-xl md:text-2xl font-bold text-[var(--text-primary)]">用户管理</h1>
          <p className="text-[var(--text-secondary)] mt-1 text-sm md:text-base">管理系统注册用户、角色分配及账户余额</p>
        </div>
      </div>

      <p className="text-xs text-[var(--text-secondary)]">至少保留一位启用的管理员。停用用户会同时阻止其全部 Key 的新调用。</p>
      {loadError && <div role="alert" className="flex items-center gap-3 rounded-lg bg-red-50 p-3 text-sm text-red-700">
        <span>{loadError}</span><button type="button" onClick={loadUsers} className="font-bold underline">重试</button>
      </div>}

      <div className="bg-[var(--surface-card)] rounded-2xl shadow-sm border border-[var(--border-soft)] overflow-hidden">
        <div className="p-3 md:p-4 border-b border-[var(--border-soft)] bg-[var(--surface)]/50 flex items-center justify-between flex-wrap gap-3">
          <div className="relative w-full md:w-72">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-secondary)]" size={16} />
            <input
              type="text"
              placeholder="搜索用户名或 ID..."
              aria-label="搜索用户"
              value={search}
              onChange={event => setSearch(event.target.value)}
              className="w-full pl-9 pr-4 py-2 bg-[var(--surface-card)] border border-[var(--border-soft)] rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
            />
          </div>
          <div className="flex gap-4">
            <select aria-label="用户状态" value={statusFilter} onChange={event => setStatusFilter(event.target.value)}
              className="rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] px-2 py-1 text-sm">
              <option value="">全部状态</option><option value="1">启用</option><option value="0">停用</option>
            </select>
            <div className="px-3 py-1 bg-[var(--primary-lighter)] text-[var(--primary)] text-xs font-bold rounded-full">
              显示 {visibleUsers.length} / 共 {users.length} 位用户
            </div>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left min-w-[600px]">
            <thead>
              <tr className="border-b border-[var(--border-soft)]">
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider">用户信息</th>
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider">角色</th>
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider">用户状态</th>
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider">账户资金</th>
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider">计费账户</th>
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider hidden sm:table-cell">注册日期</th>
                <th className="px-3 md:px-6 py-3 md:py-4 text-xs font-bold text-[var(--text-secondary)] uppercase tracking-wider text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading ? (
                Array.from({ length: 4 }).map((_, i) => (
                  <tr key={i} className="animate-pulse">
                    <td colSpan={7} className="px-3 md:px-6 py-4 md:py-6"><div className="h-4 bg-[var(--primary-lighter)] rounded w-full"></div></td>
                  </tr>
                ))
              ) : visibleUsers.length === 0 ? <tr><td colSpan={7} className="px-6 py-8 text-center text-sm text-[var(--text-secondary)]">{loadError ? '读取失败，请重试' : '没有匹配的用户'}</td></tr> : visibleUsers.map(user => (
                <tr key={user.id} className="hover:bg-[var(--surface)] transition-colors group">
                  <td className="px-3 md:px-6 py-3 md:py-4">
                    <div className="flex items-center gap-2 md:gap-3">
                      <div className="w-8 h-8 md:w-9 md:h-9 bg-[var(--primary-lighter)] rounded-full flex items-center justify-center text-[var(--primary)]">
                        <UsersIcon size={16} />
                      </div>
                      <div>
                        <div className="text-sm font-bold text-[var(--text-primary)]">{user.username}</div>
                        <div className="text-[10px] text-[var(--text-secondary)] font-mono">ID: {user.id}</div>
                      </div>
                    </div>
                  </td>
                  <td className="px-3 md:px-6 py-3 md:py-4">
                    <div className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[10px] font-bold uppercase ${user.role === UserRole.ADMIN ? 'bg-indigo-100 text-[var(--primary)]' : 'bg-[var(--primary-lighter)] text-[var(--text-secondary)]'}`}>
                      <Shield size={10} />
                      {user.role === UserRole.ADMIN ? '管理员' : '用户'}
                    </div>
                  </td>
                  <td className="px-3 md:px-6 py-3 md:py-4">
                    <span className={`text-xs font-bold ${user.status === 1 ? 'text-emerald-700' : 'text-red-700'}`}>{user.status === 1 ? '启用' : '停用'}</span>
                  </td>
                  <td className="px-3 md:px-6 py-3 md:py-4">
                    {user.billingAccount ? <div className="space-y-1 text-xs">
                      <div className="font-bold">可用 {formatFundsAmount(user.billingAccount.available)} {user.billingAccount.currency}</div>
                      <div>账面 {formatFundsAmount(user.billingAccount.posted_balance)}</div>
                      <div>预扣 {formatFundsAmount(user.billingAccount.held_amount)} · 授信 {formatFundsAmount(user.billingAccount.credit_limit)}</div>
                    </div> : <span className="text-xs text-[var(--text-secondary)]">未开通</span>}
                  </td>
                  <td className="px-3 md:px-6 py-3 md:py-4">
                    {user.billingAccount ? <button type="button" onClick={() => setBillingTarget(user)}
                      className={`rounded-lg px-2.5 py-1.5 text-xs font-bold whitespace-nowrap ${user.billingAccount.status === 'frozen' ? 'bg-red-50 text-red-700' : 'bg-[var(--primary-lighter)] text-[var(--text-secondary)]'}`}>
                      {user.billingAccount.status === 'frozen' ? '已冻结 · 查看与解冻' : user.billingAccount.status === 'closed' ? '已关闭 · 详情' : '正常 · 详情'}
                    </button> : <span className="text-xs text-[var(--text-secondary)]">未开通</span>}
                  </td>
                  <td className="px-3 md:px-6 py-3 md:py-4 text-sm text-[var(--text-secondary)] hidden sm:table-cell">
                    <div className="flex items-center gap-1.5">
                      <Calendar size={14} className="text-gray-300" />
                      {user.createdAt}
                    </div>
                  </td>
                  <td className="px-3 md:px-6 py-3 md:py-4 text-right">
                    <div className="mb-2 flex justify-end gap-3 text-xs text-[var(--primary)]">
                      <Link to={`/tokens?user_id=${user.id}`}>管理 Key</Link>
                    </div>
                    <div className="flex items-center justify-end gap-2">
                      <button
                          className="px-2 md:px-3 py-1.5 bg-[var(--surface-card)] border border-[var(--border-soft)] text-xs font-bold text-[var(--text-secondary)] rounded-lg hover:border-emerald-600 hover:text-emerald-600 transition-colors flex items-center gap-1"
                          onClick={() => {
                              setRechargeTarget(user);
                          }}
                      >
                          <Plus size={12}/>
                          账户充值
                      </button>
                        <button
                        className="px-2 md:px-3 py-1.5 bg-[var(--surface-card)] border border-[var(--border-soft)] text-xs font-bold text-[var(--text-secondary)] rounded-lg hover:border-indigo-600 hover:text-[var(--primary)] transition-colors flex items-center gap-1 disabled:opacity-50"
                        onClick={() => handleRoleChange(user)}
                        disabled={Boolean(updatingUserID) || isLastAdministrator(user)}
                        title={isLastAdministrator(user) ? '最后一位启用的管理员不能降级' : undefined}
                      >
                        <Shield size={12} />
                        <span className="hidden md:inline">{user.role === UserRole.ADMIN ? '设为用户' : '设为管理员'}</span>
                        <span className="md:hidden">角色</span>
                      </button>
                      <button type="button" onClick={() => handleStatusChange(user)}
                        disabled={Boolean(updatingUserID) || isLastAdministrator(user)}
                        title={isLastAdministrator(user) ? '最后一位启用的管理员不能停用' : undefined}
                        className="rounded-lg border border-[var(--border-soft)] px-2 py-1.5 text-xs font-bold whitespace-nowrap disabled:opacity-50">
                        {updatingUserID === user.id ? '处理中…' : user.status === 1 ? '停用用户' : '恢复用户'}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {rechargeTarget && <FundsAdjustmentModal key={rechargeTarget.id} kind="account" targetID={rechargeTarget.id}
        name={rechargeTarget.username} currency={rechargeTarget.billingAccount?.currency}
        onSubmit={adjustment => rechargeUser(rechargeTarget.id, adjustment)} onUpdated={loadUsers} onClose={() => setRechargeTarget(null)} />}
      {billingTarget && <BillingAccountModal key={billingTarget.id} user={billingTarget} onClose={() => setBillingTarget(null)} onUpdated={loadUsers} />}
    </div>
  );
};

export default Users;
