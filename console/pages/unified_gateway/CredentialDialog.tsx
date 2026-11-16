import React, { useRef, useState } from 'react';
import { PowerOff, Save } from 'lucide-react';
import { Button, Modal, useAppDialog } from '../../components/ui';
import {
  createManagedCredentialForChannel,
  managedCredentialTransition,
  transitionManagedCredential,
  updateManagedCredential,
  type CredentialPurpose,
  type CredentialCreateResult,
} from '../../services/unifiedCredentialApi';
import type { UnifiedCredential } from '../../services/unifiedGatewayApi';
import {
  ConcurrencyField as LimitField,
  configurationInputClass as inputClass,
} from './ConfigurationFields';
import { ErrorNotice, errorMessage } from './Feedback';
import { CredentialRemovalDialog } from './CredentialRemovalDialog';

const purposes: { value: CredentialPurpose; label: string }[] = [
  { value: 'execution', label: '模型调用' },
  { value: 'catalog_discovery', label: '目录发现' },
  { value: 'availability_read', label: '可用性读取' },
  { value: 'upstream_callback_verify', label: '回调验签' },
];

export const CredentialDialog: React.FC<{
  item?: UnifiedCredential;
  channelId?: number;
  poolId?: number;
  readOnly: boolean;
  onClose: () => void;
  onSaved: (created?: CredentialCreateResult) => void;
}> = ({ item, channelId, poolId, readOnly, onClose, onSaved }) => {
  const { askConfirmation } = useAppDialog();
  const [code, setCode] = useState(item?.credential_code ?? '');
  const [secret, setSecret] = useState('');
  const [weight, setWeight] = useState(String(item?.weight ?? 1));
  const [requests, setRequests] = useState(
    item?.request_limit?.toString() ?? '',
  );
  const [tasks, setTasks] = useState(item?.task_limit?.toString() ?? '');
  const [selected, setSelected] = useState<CredentialPurpose[]>(['execution']);
  const [useExistingPool, setUseExistingPool] = useState(false);
  const [pending, setPending] = useState(false);
  const locked = useRef(false);
  const [error, setError] = useState('');
  const [removing, setRemoving] = useState(false);
  const transition = item ? managedCredentialTransition(item.status) : null;
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (locked.current || readOnly) return;
    const fields = {
      request_limit: requests === '' ? null : Number(requests),
      task_limit: tasks === '' ? null : Number(tasks),
      weight: Number(weight),
    };
    if (
      Object.values(fields).some(
        (value) =>
          value !== null &&
          (!Number.isSafeInteger(value) || value < 1 || value > 1000000),
      )
    ) {
      setError('权重及并发数须为 1 到 1000000 的整数');
      return;
    }
    if (
      !item &&
      (!channelId ||
        (useExistingPool && !poolId) ||
        !/^[a-z][a-z0-9_.-]{0,127}$/.test(code) ||
        !/^[\x21-\x7e]{1,8192}$/.test(secret) ||
        selected.length === 0)
    ) {
      setError('请填写有效标识、密钥及用途');
      return;
    }
    locked.current = true;
    setPending(true);
    setError('');
    try {
      let created: CredentialCreateResult | undefined;
      if (item) {
        await updateManagedCredential(item, {
          ...fields,
          ...(secret ? { secret } : {}),
        });
      }
      else
        created = await createManagedCredentialForChannel(channelId!, {
          ...fields,
          credential_code: code,
          secret,
          purposes: selected,
        }, useExistingPool ? poolId : undefined);
      setSecret('');
      onSaved(created);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      locked.current = false;
      setPending(false);
    }
  };
  const changeStatus = async () => {
    if (locked.current || readOnly || !item || !transition) return;
    locked.current = true;
    const confirmed = await askConfirmation({
      title: transition.label,
      description: item.status === 'active'
        ? '停止使用后，此 Key 不再接收新调用。'
        : '完成停用后，此 Key 将不可再使用。',
      confirmLabel: transition.label,
      tone: item.status === 'active' ? 'warning' : 'danger',
    });
    if (!confirmed) {
      locked.current = false;
      return;
    }
    setPending(true);
    setError('');
    try {
      await transitionManagedCredential(item);
      onSaved();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      locked.current = false;
      setPending(false);
    }
  };
  if (removing && item) return <CredentialRemovalDialog item={item} readOnly={readOnly} onClose={() => setRemoving(false)} onSaved={() => { setSecret(''); onSaved(); }} />;
  return (
    <Modal
      open
      title={item ? '编辑API Key' : '新建API Key'}
      onClose={() => {
        if (!pending) onClose();
      }}
    >
      <form onSubmit={submit} className="space-y-4" autoComplete="off">
        {error && <ErrorNotice message={error} />}
        <fieldset
          disabled={pending || readOnly || Boolean(item && item.status !== 'active')}
          className="min-w-0 space-y-4"
        >
          {!item && <div className="space-y-2 text-xs leading-5 text-[var(--text-secondary)]">
            <p>默认建立独立分组，不继承其他 Key 的模型或价格。</p>
            {poolId && <label className="flex items-start gap-2">
              <input type="checkbox" checked={useExistingPool} onChange={(event) => setUseExistingPool(event.target.checked)} className="mt-1 h-4 w-4 shrink-0 accent-[var(--primary)]" />
              <span>加入当前分组（确认价格、权限和服务规格一致）</span>
            </label>}
          </div>}
          <label className="block space-y-2 text-sm font-semibold">
            <span>Key 名称</span>
            <input
              className={`${inputClass} font-mono`}
              required
              maxLength={128}
              value={code}
              disabled={Boolean(item)}
              onChange={(event) => setCode(event.target.value)}
            />
          </label>
          <>
              <label className="block space-y-2 text-sm font-semibold">
                <span>{item ? '替换密钥（可选）' : '密钥'}</span>
                <input
                  type="password"
                  autoComplete="new-password"
                  spellCheck={false}
                  className={inputClass}
                  required={!item}
                  maxLength={8192}
                  value={secret}
                  onChange={(event) => setSecret(event.target.value)}
                />
              </label>
              {!item && <fieldset className="space-y-2">
                <legend className="mb-2 text-sm font-semibold">用途</legend>
                <div className="flex flex-wrap gap-4">
                  {purposes.map((purpose) => (
                    <label
                      key={purpose.value}
                      className="flex items-center gap-2 text-sm"
                    >
                      <input
                        type="checkbox"
                        className="h-4 w-4 accent-[var(--primary)]"
                        checked={selected.includes(purpose.value)}
                        onChange={(event) =>
                          setSelected((current) =>
                            event.target.checked
                              ? [...current, purpose.value]
                              : current.filter(
                                  (value) => value !== purpose.value,
                                ),
                          )
                        }
                      />
                      {purpose.label}
                    </label>
                  ))}
                </div>
              </fieldset>}
          </>
          <label className="block space-y-2 text-sm font-semibold">
            <span>权重</span>
            <input
              className={inputClass}
              type="number"
              min={1}
              max={1000000}
              step={1}
              required
              value={weight}
              onChange={(event) => setWeight(event.target.value)}
            />
          </label>
          <div className="grid gap-4 sm:grid-cols-2">
            <LimitField
              label="请求并发"
              value={requests}
              onChange={setRequests}
              pending={pending || readOnly}
            />
            <LimitField
              label="任务并发"
              value={tasks}
              onChange={setTasks}
              pending={pending || readOnly}
            />
          </div>
        </fieldset>
        {item?.status === 'draining' && (
          <p className="text-xs leading-5 text-[var(--text-secondary)]">
            此 Key 已停止接收新调用，确认没有进行中的调用后可完成停用。
          </p>
        )}
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-[var(--border-soft)] pt-4">
          <div className="flex gap-2">
            {item && transition && (
              <Button
                type="button"
                variant="danger"
                loading={pending}
                disabled={readOnly}
                onClick={() => void changeStatus()}
              >
                <PowerOff size={15} />
                {transition.label}
              </Button>
            )}
            {item && (
              <Button type="button" variant="danger" disabled={readOnly || pending} onClick={() => setRemoving(true)}>
                <PowerOff size={15} />
                移除 Key
              </Button>
            )}
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={onClose}
            >
              取消
            </Button>
            {(!item || item.status === 'active') && (
              <Button type="submit" loading={pending} disabled={readOnly}>
                <Save size={15} />
                保存
              </Button>
            )}
          </div>
        </div>
      </form>
    </Modal>
  );
};
