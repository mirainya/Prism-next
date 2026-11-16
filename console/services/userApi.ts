import { BillingAccountDetail, FundsAdjustment, User, UserRole } from '../types';
import { request } from './request';

export const fetchUsers = async (): Promise<User[]> => {
  const data = await request<any[]>('/admin/users');
  return data.map(u => ({
    id: String(u.id),
    username: u.username,
    role: u.role as UserRole,
    balance: Number(u.balance) || 0,
    createdAt: new Date(u.created_at).toISOString().split('T')[0],
    status: u.status,
    billingAccount: u.billing_account ?? null,
  }));
};

export const updateUserRole = async (id: string, role: UserRole): Promise<void> => {
  await request(`/admin/users/${id}/role`, {
    method: 'PUT',
    body: JSON.stringify({ role }),
  });
};

export const fetchUserBillingAccount = (id: string, signal?: AbortSignal): Promise<BillingAccountDetail> =>
  request(`/admin/users/${encodeURIComponent(id)}/billing-account`, { signal });

export const unfreezeUserBillingAccount = (id: string, freezeEventID: number, note: string): Promise<{ updated: boolean; status: 'open' }> =>
  request(`/admin/users/${encodeURIComponent(id)}/billing-account/unfreeze`, {
    method: 'POST',
    body: JSON.stringify({ expected_freeze_event_id: freezeEventID, note: note.trim() }),
  });

export const updateUserStatus = async (id: string, status: number): Promise<void> => {
  await request(`/admin/users/${id}/status`, {
    method: 'PUT',
    body: JSON.stringify({ status }),
  });
};

export const rechargeUser = async (id: string, adjustment: FundsAdjustment): Promise<void> => {
    await request(`/admin/users/${id}/recharge`, {
        method: 'POST',
        body: JSON.stringify(adjustment),
    });
};
