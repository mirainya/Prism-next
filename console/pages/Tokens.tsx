import React, { useEffect, useRef, useState } from 'react';
import {
    Key,
    Plus,
    Copy,
    CheckCircle2,
    AlertCircle,
    HardDriveUpload,
    Unlink,
} from 'lucide-react';
import { Button, Modal, Select, useAppDialog } from '../components/ui';
import { PageHeader } from '../components/shell';
import {
    fetchMyBillingAccount,
    createToken,
    rechargeToken,
    updateToken,
    bindTokenFileStorage,
    fetchTokenFileStorageEndpoints,
    unbindTokenFileStorage,
} from '../services/api';
import { ApiToken, BillingAccountSummary } from '../types';
import { TokenManagementList } from '../components/users/TokenManagementList';
import { fetchTokenRateSettings, TokenRateSettings } from '../services/tokenManagementApi';
import { expirationISO, TokenPolicyFields, validTokenPolicy } from '../components/users/TokenPolicyFields';
import { FundsAdjustmentModal, formatFundsAmount, formatFundsDisplay } from '../components/users/FundsAdjustmentModal';

const Tokens: React.FC = () => {
  const { askConfirmation, showAlert } = useAppDialog();
  const [revision, setRevision] = useState(0);
  const [loadError, setLoadError] = useState('');
  const [account, setAccount] = useState<BillingAccountSummary | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newTokenName, setNewTokenName] = useState('');
  const [newTokenRate, setNewTokenRate] = useState('0');
  const [newTokenExpiration, setNewTokenExpiration] = useState('');
  const [rateSettings, setRateSettings] = useState<TokenRateSettings | null>(null);
    const [newTokenBalance, setNewTokenBalance] = useState<string>('');
  const [newTokenKey, setNewTokenKey] = useState('');
  const [isCreating, setIsCreating] = useState(false);

  const [budgetTarget, setBudgetTarget] = useState<ApiToken | null>(null);

    // 编辑相关状态
    const [showEditModal, setShowEditModal] = useState(false);
    const [editTokenId, setEditTokenId] = useState<string>('');
    const [editTokenName, setEditTokenName] = useState<string>('');
    const [isEditing, setIsEditing] = useState(false);

    // 每个 Prism API Key 独立选择结果转存账号。
    const [storageToken, setStorageToken] = useState<ApiToken | null>(null);
    const [storageBaseURL, setStorageBaseURL] = useState('');
    const [storageEndpoints, setStorageEndpoints] = useState<string[]>([]);
    const [isLoadingStorageEndpoints, setIsLoadingStorageEndpoints] = useState(false);
    const [storageEndpointsError, setStorageEndpointsError] = useState(false);
    const storageRequestID = useRef(0);
    const [storageAPIKey, setStorageAPIKey] = useState('');
    const [isSavingStorage, setIsSavingStorage] = useState(false);

  const loadTokens = () => {
    setRevision(value => value + 1);
    setLoadError('');
    fetchMyBillingAccount().then(setAccount).catch(err => setLoadError(err.message || '读取账户资金失败'));
    fetchTokenRateSettings().then(setRateSettings).catch(err => setLoadError(err.message || '读取限流设置失败'));
  };

  useEffect(() => { loadTokens(); }, []);

  const copyText = (text: string) => {
    if (navigator.clipboard?.writeText) {
      return navigator.clipboard.writeText(text);
    }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
    return Promise.resolve();
  };

  const handleCreate = async () => {
    if (!newTokenName.trim() || !validTokenPolicy(newTokenRate, newTokenExpiration)) return;
    setIsCreating(true);
    try {
        const balance = newTokenBalance.trim() || '0';
        const result = await createToken(newTokenName, balance, { rate_limit: Number(newTokenRate), expires_at: expirationISO(newTokenExpiration) });
      setNewTokenKey(result.key);
      loadTokens();
    } catch (err: any) {
      await showAlert({ title: '创建失败', description: err.message || 'API Key 创建失败，请稍后重试。', tone: 'danger' });
    } finally {
      setIsCreating(false);
    }
  };

    const openRechargeModal = (token: ApiToken) => {
        setBudgetTarget(token);
    };

    // 打开编辑弹窗
    const openEditModal = (token: ApiToken) => {
        setEditTokenId(token.id);
        setEditTokenName(token.name);
        setShowEditModal(true);
    };

    // 保存编辑
    const handleSaveEdit = async () => {
        setIsEditing(true);
        try {
            await updateToken(editTokenId, {
                name: editTokenName,
            });
            loadTokens();
            setShowEditModal(false);
        } catch (err: any) {
            await showAlert({ title: '保存失败', description: err.message || 'API Key 保存失败，请稍后重试。', tone: 'danger' });
        } finally {
            setIsEditing(false);
        }
    };

    const openStorageModal = (token: ApiToken) => {
        const requestID = ++storageRequestID.current;
        setStorageToken(token);
        setStorageBaseURL(token.xfsStorage.baseURL);
        setStorageEndpoints([]);
        setStorageEndpointsError(false);
        setIsLoadingStorageEndpoints(true);
        setStorageAPIKey('');
        fetchTokenFileStorageEndpoints()
            .then(endpoints => {
                if (requestID !== storageRequestID.current) return;
                setStorageEndpoints(endpoints);
                setStorageBaseURL(current => endpoints.includes(current) ? current : endpoints[0] || '');
                setStorageEndpointsError(endpoints.length === 0);
            })
            .catch(() => {
                if (requestID === storageRequestID.current) setStorageEndpointsError(true);
            })
            .finally(() => {
                if (requestID === storageRequestID.current) setIsLoadingStorageEndpoints(false);
            });
    };

    const closeStorageModal = () => {
        if (isSavingStorage) return;
        storageRequestID.current += 1;
        setStorageToken(null);
        setStorageBaseURL('');
        setStorageEndpoints([]);
        setStorageAPIKey('');
    };

    const handleBindStorage = async () => {
        if (!storageToken || !storageAPIKey.trim() || !storageEndpoints.includes(storageBaseURL)) return;
        setIsSavingStorage(true);
        try {
            await bindTokenFileStorage(storageToken.id, storageBaseURL, storageAPIKey.trim());
            loadTokens();
            storageRequestID.current += 1;
            setStorageToken(null);
            setStorageBaseURL('');
            setStorageAPIKey('');
        } catch (err: any) {
            await showAlert({
                title: '绑定失败',
                description: err.message || 'XFileStorage Key 验证失败，请检查 Key 与服务状态。',
                tone: 'danger',
            });
        } finally {
            setIsSavingStorage(false);
        }
    };

    const handleUnbindStorage = async () => {
        if (!storageToken) return;
        const confirmed = await askConfirmation({
            title: '停止新任务转存？',
            description: `解绑后，“${storageToken.name}”的图片和视频结果将直接使用上游地址。`,
            confirmLabel: '确认解绑',
            tone: 'danger',
        });
        if (!confirmed) return;
        setIsSavingStorage(true);
        try {
            await unbindTokenFileStorage(storageToken.id);
            loadTokens();
            storageRequestID.current += 1;
            setStorageToken(null);
            setStorageBaseURL('');
            setStorageAPIKey('');
        } catch (err: any) {
            await showAlert({ title: '解绑失败', description: err.message || '暂时无法解绑，请稍后重试。', tone: 'danger' });
        } finally {
            setIsSavingStorage(false);
        }
    };

    const closeModal = () => {
        setShowCreateModal(false);
        setNewTokenName('');
        setNewTokenRate('0');
        setNewTokenExpiration('');
        setNewTokenBalance('');
        setNewTokenKey('');
    };

  return (
    <div className="space-y-4">
      {loadError && <p role="alert" className="text-sm text-red-700">{loadError}</p>}
      <PageHeader
        icon={Key}
        title="API Key"
        meta="管理调用密钥与消费额度"
        actions={(
          <Button onClick={() => setShowCreateModal(true)}>
            <Plus size={16} />
            创建 API Key
          </Button>
        )}
      />
      <div className="rounded-lg border border-[var(--border-soft)] bg-[var(--surface-card)] p-4 text-sm">
        {account ? <><p className="text-xs text-[var(--text-secondary)]">我的账户 · {account.status === 'frozen' ? '已冻结' : account.status === 'closed' ? '已关闭' : '正常'}</p>
          <p className="mt-1">可用额度 <strong className="ml-2 text-xl tabular-nums" title={formatFundsAmount(account.available)}>{formatFundsDisplay(account.available)}</strong> <span className="text-xs text-[var(--text-secondary)]">{account.currency}</span></p>
          <details className="mt-2 text-xs text-[var(--text-secondary)]"><summary className="w-fit cursor-pointer">账户明细</summary><p className="mt-2">账面余额 {formatFundsAmount(account.posted_balance)} · 处理中占用 {formatFundsAmount(account.held_amount)} · 授信额度 {formatFundsAmount(account.credit_limit)}</p></details>
        </> : <p>账户信息暂未读取</p>}
        <p className="mt-2 text-xs text-[var(--text-secondary)]">同一用户的 Key 共用账户资金，每把 Key 可单独设置消费上限。</p>
      </div>

      <TokenManagementList revision={revision} rateSettings={rateSettings} currency={account?.currency} onBudget={openRechargeModal} onEdit={openEditModal} onStorage={openStorageModal} />

        {/* 创建令牌弹窗 */}
        <Modal open={showCreateModal} onClose={closeModal} title="创建 API Key" width="max-w-xl">
            {newTokenKey ? (
              <div className="modal-form">
                <div className="modal-scroll-body space-y-4">
                <div className="p-4 bg-amber-50 border border-amber-300 rounded-xl">
                    <div className="flex items-center gap-2 mb-2">
                        <AlertCircle size={16} className="text-amber-600" />
                        <p className="text-sm font-bold text-amber-700">请立即复制并妥善保存，关闭后将无法再次查看！</p>
                    </div>
                </div>
                <div className="p-4 bg-green-50 border border-green-200 rounded-xl">
                    <p className="text-sm text-green-700 mb-2 font-medium">API Key 创建成功</p>
                    <textarea
                      readOnly
                      value={newTokenKey}
                      rows={3}
                      onClick={e => (e.target as HTMLTextAreaElement).select()}
                      className="w-full text-sm font-mono bg-[var(--surface-card)] p-3 rounded-lg border border-green-200 resize-none focus:outline-none focus:ring-2 focus:ring-green-400"
                    />
                    <button
                      onClick={() => {
                        copyText(newTokenKey).then(() => {
                          setCopiedId('new');
                          setTimeout(() => setCopiedId(null), 2000);
                        });
                      }}
                      className="modal-button modal-button-primary mt-2 w-full"
                    >
                      {copiedId === 'new' ? (
                        <><CheckCircle2 size={16} /> 已复制</>
                      ) : (
                        <><Copy size={16} /> 复制完整 API Key</>
                      )}
                    </button>
                </div>
                </div>
                <div className="modal-footer">
                  <button onClick={closeModal} className="modal-button modal-button-secondary">我已保存，关闭</button>
                </div>
              </div>
            ) : (
              <div className="modal-form">
               <div className="modal-scroll-body space-y-4">
                <div>
                  <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">名称</label>
                  <input
                    type="text"
                    value={newTokenName}
                    onChange={e => setNewTokenName(e.target.value)}
                    placeholder="如: 生产环境、测试项目"
                    className="w-full px-4 py-3 border border-[var(--border-soft)] rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
                  />
                </div>
                  <div>
                      <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">消费上限{account?.currency ? `（${account.currency}）` : ''}</label>
                      <input
                          type="number"
                          step="0.01"
                          min="0"
                          value={newTokenBalance}
                          onChange={e => setNewTokenBalance(e.target.value)}
                          placeholder="0.00"
                          className="w-full px-4 py-3 border border-[var(--border-soft)] rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
                  />
                </div>
                <TokenPolicyFields rate={newTokenRate} onRate={setNewTokenRate} expiration={newTokenExpiration} onExpiration={setNewTokenExpiration} settings={rateSettings} disabled={isCreating} prefix="create-token" />
                </div>
                <div className="modal-footer">
                  <button type="button" onClick={closeModal} className="modal-button modal-button-secondary">取消</button>
                  <button onClick={handleCreate} disabled={isCreating || !newTokenName.trim() || !validTokenPolicy(newTokenRate, newTokenExpiration)} className="modal-button modal-button-primary">
                  {isCreating ? (
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white"></div>
                  ) : (
                    <>
                      <Plus size={18} />
                      创建 API Key
                    </>
                  )}
                  </button>
                </div>
              </div>
            )}
        </Modal>

      {budgetTarget && <FundsAdjustmentModal key={budgetTarget.id} kind="budget" targetID={budgetTarget.id} name={budgetTarget.name}
        currency={account?.currency} onSubmit={adjustment => rechargeToken(budgetTarget.id, adjustment)} onUpdated={loadTokens} onClose={() => setBudgetTarget(null)} />}

        <Modal open={storageToken !== null} onClose={closeStorageModal} title="文件存储设置" width="max-w-lg">
          <div className="modal-form">
            <div className="modal-scroll-body space-y-4">
              <div className="rounded-xl bg-[var(--surface)] p-4">
                <p className="text-xs text-[var(--text-secondary)]">API Key</p>
                <p className="mt-1 font-bold text-[var(--text-primary)]">{storageToken?.name}</p>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">
                  绑定后，此 Key 的生成结果会自动保存到所选文件存储服务。
                </p>
                <p className="mt-2 text-xs text-[var(--text-secondary)]">
                  输入素材和文件也使用所选服务；未绑定时使用系统全局存储，生成结果则直接返回上游地址。
                </p>
              </div>
              {storageToken?.xfsStorage.configured && (
                <div className="flex items-center justify-between gap-3 rounded-xl border border-green-200 bg-green-50 p-3 text-sm">
                  <span className="min-w-0 font-semibold text-green-700">当前已绑定 {storageToken.xfsStorage.keyHint}<span className="mt-1 block break-all text-xs font-normal">{storageToken.xfsStorage.baseURL}</span></span>
                  <button
                    type="button"
                    onClick={() => void handleUnbindStorage()}
                    disabled={isSavingStorage}
                    className="inline-flex items-center gap-1 font-semibold text-red-600 disabled:opacity-50"
                  >
                    <Unlink size={14} />
                    解绑
                  </button>
                </div>
              )}
              <div>
                <label className="mb-1 block text-sm font-medium text-[var(--text-primary)]">XFileStorage 服务</label>
                <Select
                  value={storageBaseURL}
                  onChange={setStorageBaseURL}
                  options={storageEndpoints.map(baseURL => ({ value: baseURL, label: baseURL }))}
                  placeholder={isLoadingStorageEndpoints ? '正在加载' : '选择服务'}
                  disabled={isLoadingStorageEndpoints || storageEndpoints.length === 0 || isSavingStorage}
                />
                {storageEndpointsError && <p className="mt-2 text-xs text-red-600">服务列表暂不可用</p>}
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-[var(--text-primary)]">
                  {storageToken?.xfsStorage.configured ? '新的 XFileStorage Key' : 'XFileStorage Key'}
                </label>
                <input
                  type="text"
                  value={storageAPIKey}
                  onChange={event => setStorageAPIKey(event.target.value)}
                  placeholder="xfs_..."
                  autoComplete="off"
                  spellCheck={false}
                  className="w-full rounded-lg border border-[var(--border-soft)] px-4 py-3 font-mono text-sm focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
                />
                <p className="mt-2 text-xs text-[var(--text-secondary)]">保存时会验证上传、读取、签名和删除权限。</p>
              </div>
            </div>
            <div className="modal-footer">
              <button type="button" onClick={closeStorageModal} disabled={isSavingStorage} className="modal-button modal-button-secondary">取消</button>
              <button
                type="button"
                onClick={() => void handleBindStorage()}
                disabled={isSavingStorage || !storageAPIKey.trim() || !storageEndpoints.includes(storageBaseURL)}
                className="modal-button modal-button-primary"
              >
                <HardDriveUpload size={17} />
                {isSavingStorage ? '正在验证' : storageToken?.xfsStorage.configured ? '验证并更换' : '验证并绑定'}
              </button>
            </div>
          </div>
        </Modal>

        {/* 编辑令牌弹窗 */}
            <Modal open={showEditModal} onClose={() => setShowEditModal(false)} title="修改 Key 名称" width="max-w-xl">
                    <div className="modal-form">
                      <div className="modal-scroll-body space-y-4">
                        <div>
                            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">名称</label>
                            <input
                                type="text"
                                value={editTokenName}
                                onChange={e => setEditTokenName(e.target.value)}
                                placeholder="Key 名称"
                                className="w-full px-4 py-3 border border-[var(--border-soft)] rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
                            />
                        </div>
                      </div>
                      <div className="modal-footer">
                        <button type="button" onClick={() => setShowEditModal(false)} className="modal-button modal-button-secondary">取消</button>
                        <button onClick={handleSaveEdit} disabled={isEditing || !editTokenName.trim()} className="modal-button modal-button-primary">
                            {isEditing ? (
                                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white"></div>
                            ) : (
                                <>
                                    <CheckCircle2 size={18}/>
                                    保存
                                </>
                            )}
                        </button>
                      </div>
                    </div>
            </Modal>
    </div>
  );
};

export default Tokens;
