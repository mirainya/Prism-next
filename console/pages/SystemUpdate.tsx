import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { CheckCircle2, DownloadCloud, Plus, RefreshCw, Save, Server, Trash2 } from 'lucide-react';
import { Badge, Button, Card, Input } from '../components/ui';
import { PageHeader } from '../components/shell';
import {
  checkSystemUpdate,
  downloadSystemUpdate,
  fetchSystemUpdateStatus,
  fetchSystemVersion,
  fetchUpdateSources,
  saveUpdateSources,
  prepareSystemUpdate,
  installSystemUpdate,
  type UpdateInstallPlan,
  type UpdateCheckResult,
  type UpdateDownloadResult,
  type UpdateSource,
  type UpdateSourceInput,
  type VersionInfo,
} from '../services/systemUpdateApi';

const sourceKey = (source: UpdateSource, index: number) => source.built_in ? 'built-in-github' : source.id ? `source-${source.id}` : `new-${index}`;
const formatBytes = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) return '-';
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(2)} MB`;
};
const formatTime = (value?: string) => value && !Number.isNaN(new Date(value).getTime()) ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : value || '-';
const fileName = (value?: string) => value ? value.split(/[\\/]/).pop() || value : '-';
const stateLabels: Record<UpdateDownloadResult['state'], string> = {
  idle: '等待更新', downloading: '更新包下载中', staged: '更新包已校验', failed: '更新失败',
  queued: '等待安装', installing: '正在安装', completed: '更新完成', rolled_back: '已恢复旧程序', recovery_required: '需要恢复处理',
};

const SystemUpdate: React.FC = () => {
  const [version, setVersion] = useState<VersionInfo | null>(null);
  const [sources, setSources] = useState<UpdateSource[]>([]);
  const [sourceRevision, setSourceRevision] = useState(0);
  const [check, setCheck] = useState<UpdateCheckResult | null>(null);
  const [download, setDownload] = useState<UpdateDownloadResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [checking, setChecking] = useState(false);
  const [saving, setSaving] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [installPlan, setInstallPlan] = useState<UpdateInstallPlan | null>(null);
  const [preparing, setPreparing] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError('');
    try {
      const [nextVersion, nextSources, nextStatus] = await Promise.all([
        fetchSystemVersion(controller.signal),
        fetchUpdateSources(controller.signal),
        fetchSystemUpdateStatus(controller.signal),
      ]);
      setVersion(nextVersion);
      setSources(nextSources.sources);
      setSourceRevision(nextSources.revision);
      setDownload(nextStatus.state === 'idle' ? null : nextStatus);
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : '读取更新配置失败');
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    return () => {
      abortRef.current?.abort();
    };
  }, [load]);

  useEffect(() => {
    if (!download || !['downloading', 'queued', 'installing'].includes(download.state)) return;
    const controller = new AbortController();
    let timer = 0;
    const poll = async () => {
      try {
        const status = await fetchSystemUpdateStatus(controller.signal);
        if (controller.signal.aborted) return;
        if (status.state === 'completed') {
          setVersion(await fetchSystemVersion(controller.signal));
          if (controller.signal.aborted) return;
        }
        setDownload(status.state === 'idle' ? null : status);
        if (['downloading', 'queued', 'installing'].includes(status.state)) timer = window.setTimeout(poll, 2000);
        if (status.state === 'staged') setNotice('更新包已下载并完成校验，尚未替换当前程序');
        if (status.state === 'failed') setError(status.error || status.message || '更新失败');
        if (status.state === 'completed') {
          setNotice('更新完成，刷新页面加载新版界面');
          setInstallPlan(null);
          setCheck(null);
        }
        if (status.state === 'rolled_back' || status.state === 'recovery_required') setError(status.message || '安装失败，请查看服务器更新日志');
      } catch (cause) {
        if (!controller.signal.aborted) {
          if (download.state === 'downloading') setError(cause instanceof Error ? cause.message : '读取下载状态失败');
          else setNotice('更新服务正在重启，等待重新连接');
          timer = window.setTimeout(poll, 5000);
        }
      }
    };
    timer = window.setTimeout(poll, 1000);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [download?.state]);

  useEffect(() => { setInstallPlan(null); setConfirmed(false); }, [download?.version, download?.sha256]);

  const updateBusy = preparing || installing || downloading || ['downloading', 'queued', 'installing', 'recovery_required'].includes(download?.state || '');

  const reviewInstall = async () => {
    setPreparing(true); setError(''); setNotice(''); setConfirmed(false);
    try { setInstallPlan(await prepareSystemUpdate()); }
    catch (cause) { setError(cause instanceof Error ? cause.message : '读取安装计划失败'); }
    finally { setPreparing(false); }
  };

  const runInstall = async () => {
    if (!installPlan || !confirmed) return;
    setInstalling(true); setError(''); setNotice('');
    try {
      setDownload(await installSystemUpdate(installPlan.version, installPlan.plan.digest));
      setNotice('更新已提交，安装和重启由独立更新服务执行');
    } catch (cause) { setError(cause instanceof Error ? cause.message : '提交更新失败'); }
    finally { setInstalling(false); }
  };

  const enabledSources = useMemo(() => sources.filter(source => source.enabled && source.base_url.trim()), [sources]);

  const updateSource = (index: number, patch: Partial<UpdateSource>) => {
    setSources(current => current.map((source, itemIndex) => itemIndex === index ? { ...source, ...patch } : source));
  };

  const addSource = () => {
    setSources(current => [...current, { name: '', base_url: '', enabled: true, priority: current.length + 1 }]);
    setNotice('已添加空白更新源，请填写地址后保存');
    setError('');
  };

  const removeSource = (index: number) => {
    setSources(current => current.filter((_, itemIndex) => itemIndex !== index));
    setNotice('已移除，点击“保存来源”后生效');
  };

  const saveSources = async () => {
    setError('');
    setNotice('');
    const invalid = sources.some(source => !source.name.trim() || !/^https:\/\//i.test(source.base_url.trim()));
    if (invalid) {
      setError('每个更新源都需要填写名称和 HTTPS 地址');
      return;
    }
    if (enabledSources.length === 0) {
      setError('至少保留一个启用的更新源');
      return;
    }
    setSaving(true);
    try {
      const payload: UpdateSourceInput[] = sources.map(source => ({
        ...(source.id ? { id: source.id } : {}),
        ...(source.config_version != null ? { config_version: source.config_version } : {}),
        ...(source.built_in ? { built_in: true } : {}),
        name: source.name.trim(),
        base_url: source.base_url.trim().replace(/\/+$/, ''),
        enabled: Boolean(source.enabled),
        priority: Number.isFinite(Number(source.priority)) ? Math.max(0, Number(source.priority)) : 0,
      }));
      const config = await saveUpdateSources(payload, sourceRevision);
      setSources(config.sources);
      setSourceRevision(config.revision);
      setNotice('更新源已保存');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '保存更新源失败');
    } finally {
      setSaving(false);
    }
  };

  const runCheck = async () => {
    setChecking(true);
    setError('');
    setNotice('');
    try {
      setCheck(await checkSystemUpdate());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '检查更新失败');
    } finally {
      setChecking(false);
    }
  };

  const runDownload = async () => {
    setDownloading(true);
    setError('');
    setNotice('');
    try {
      const result = await downloadSystemUpdate();
      setDownload(result);
      if (result.state === 'downloading') {
        setNotice('更新包正在下载和校验，页面会自动刷新状态');
        return;
      }
      setNotice('更新包已下载并完成校验，尚未替换当前程序');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '下载更新包失败');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader
        icon={DownloadCloud}
        title="系统更新"
        meta="管理更新来源，检查、下载并安装发布版本"
        actions={<Button variant="secondary" size="sm" onClick={load} disabled={loading || saving}><RefreshCw size={15} className={loading ? 'animate-spin' : ''} />刷新</Button>}
      />

      {(error || notice) && <div className={`rounded-lg border px-4 py-3 text-sm ${error ? 'border-red-200 bg-red-50 text-red-700' : 'border-emerald-200 bg-emerald-50 text-emerald-700'}`} role="status">{error || notice}</div>}

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,1fr)]">
        <Card hover={false} className="min-w-0">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-base font-bold text-[var(--text-primary)]">版本检查</h2>
              <p className="mt-1 text-xs text-[var(--text-secondary)]">检查只读取发布清单，不会修改程序或数据库。</p>
            </div>
            <Button size="sm" onClick={runCheck} loading={checking}><RefreshCw size={14} />检查更新</Button>
          </div>
          <div className="mt-5 grid gap-3 sm:grid-cols-3">
            <div className="min-w-0 rounded-lg border border-[var(--border-soft)] bg-[var(--surface-muted)]/40 p-3"><div className="text-xs text-[var(--text-secondary)]">当前版本</div><div className="mt-1 break-all font-mono text-lg font-bold" title={version?.version}>{version?.version || (loading ? '读取中' : '-')}</div></div>
            <div className="min-w-0 rounded-lg border border-[var(--border-soft)] bg-[var(--surface-muted)]/40 p-3"><div className="text-xs text-[var(--text-secondary)]">可用版本</div><div className="mt-1 break-all font-mono text-lg font-bold" title={check?.latest_version}>{check?.latest_version || '-'}</div></div>
            <div className="min-w-0 rounded-lg border border-[var(--border-soft)] bg-[var(--surface-muted)]/40 p-3"><div className="text-xs text-[var(--text-secondary)]">检查来源</div><div className="mt-1 break-words text-sm font-semibold [overflow-wrap:anywhere]" title={check?.source}>{check?.source || '-'}</div></div>
          </div>
          {check && <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-[var(--border-soft)] pt-4">
            {check.update_available ? <Badge variant="warning">发现新版本 v{check.latest_version}</Badge> : <Badge variant="success"><CheckCircle2 size={13} className="mr-1" />已是最新版本</Badge>}
            {check.manifest?.build_time && <span className="text-xs text-[var(--text-secondary)]">构建于 {formatTime(check.manifest.build_time)}</span>}
            {check.update_available && <Button size="sm" variant="secondary" onClick={runDownload} loading={downloading || download?.state === 'downloading'} disabled={updateBusy}><DownloadCloud size={14} />下载并校验</Button>}
          </div>}
          {download && <div className={`mt-4 rounded-lg border p-3 text-sm ${['downloading', 'queued', 'installing'].includes(download.state) ? 'border-amber-200 bg-amber-50/70 text-amber-800' : ['failed', 'recovery_required', 'rolled_back'].includes(download.state) ? 'border-red-200 bg-red-50/70 text-red-800' : 'border-emerald-200 bg-emerald-50/70 text-emerald-800'}`}>
            <div className="flex items-center gap-2 font-semibold">{['downloading', 'queued', 'installing'].includes(download.state) ? <RefreshCw size={16} className="animate-spin" /> : ['failed', 'recovery_required', 'rolled_back'].includes(download.state) ? <span aria-hidden="true">!</span> : <CheckCircle2 size={16} />}{stateLabels[download.state]}</div>
            {download.state === 'staged' && <div className="mt-2 grid gap-1 break-words text-xs [overflow-wrap:anywhere] sm:grid-cols-2"><span className="min-w-0">版本：v{download.version}</span><span className="min-w-0">文件：{fileName(download.asset?.name)}</span><span className="min-w-0">大小：{formatBytes(download.size || download.asset?.size || 0)}</span><span className="min-w-0 truncate" title={download.sha256}>SHA256：{download.sha256}</span></div>}
            {download.state === 'staged' && <div className="mt-3">
              {download.install_available ? <Button size="sm" onClick={reviewInstall} loading={preparing} disabled={updateBusy}>预览安装计划</Button> : <p className="text-xs">{download.install_reason || '服务器尚未配置独立更新服务'}</p>}
            </div>}
            {download.state === 'queued' || download.state === 'installing' ? <p className="mt-2 text-xs">正在安装并重启服务，页面将自动重新连接。</p> : null}
            {download.state === 'completed' && <div className="mt-2"><p>{download.message || '更新完成，健康检查通过'}</p><Button size="sm" variant="secondary" className="mt-2" onClick={() => window.location.reload()}>刷新页面</Button></div>}
            {(download.state === 'failed' || download.state === 'recovery_required' || download.state === 'rolled_back') && <p className="mt-2 text-xs">{download.error || download.message}</p>}
          </div>}
          {installPlan && download?.state === 'staged' && <div className="mt-4 space-y-3 rounded-lg border border-[var(--border-soft)] p-3">
            <h3 className="font-semibold">安装 v{installPlan.version}</h3>
            <p className="text-xs text-[var(--text-secondary)]">服务将短暂重启。{installPlan.plan.migrations.length ? `执行以下 ${installPlan.plan.migrations.length} 项数据库迁移，执行前自动备份。` : '无需数据库迁移。'}</p>
            {installPlan.plan.migrations.map(migration => <details key={migration.Version} className="min-w-0 rounded-lg border border-[var(--border-soft)] p-2">
              <summary className="cursor-pointer break-all text-xs">{migration.Filename}</summary>
              <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-[var(--surface-muted)] p-3 text-xs">{migration.SQL}</pre>
            </details>)}
            <label className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />确认上述迁移并允许安装、重启服务</label>
            <Button size="sm" onClick={runInstall} loading={installing} disabled={!confirmed || updateBusy}>确认安装</Button>
          </div>}
        </Card>

        <Card hover={false} className="min-w-0">
          <div className="flex items-start gap-3"><Server size={20} className="mt-0.5 text-[var(--primary)]" /><div><h2 className="text-base font-bold text-[var(--text-primary)]">运行信息</h2><p className="mt-1 text-xs text-[var(--text-secondary)]">当前安装版本的构建信息。</p></div></div>
          <dl className="mt-5 space-y-3 text-sm"><div className="flex justify-between gap-3"><dt className="shrink-0 text-[var(--text-secondary)]">构建时间</dt><dd className="min-w-0 break-words text-right font-mono text-xs">{formatTime(version?.build_time)}</dd></div><div className="flex justify-between gap-3"><dt className="shrink-0 text-[var(--text-secondary)]">源码版本</dt><dd className="min-w-0 max-w-[14rem] truncate font-mono text-xs" title={version?.source_revision}>{version?.source_revision || '-'}</dd></div><div className="flex justify-between gap-3"><dt className="shrink-0 text-[var(--text-secondary)]">启用更新源</dt><dd>{enabledSources.length} 个</dd></div></dl>
        </Card>
      </div>

      <Card hover={false} className="min-w-0">
        <div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0"><h2 className="text-base font-bold text-[var(--text-primary)]">更新源</h2><p className="mt-1 text-xs text-[var(--text-secondary)]">自定义来源可修改地址、启停或删除；优先级数字越小，下载时越先尝试。</p></div><div className="flex flex-wrap gap-2"><Button variant="ghost" size="sm" onClick={addSource} disabled={loading || saving || sources.filter(source => !source.built_in).length >= 8}><Plus size={14} />添加来源</Button><Button size="sm" onClick={saveSources} loading={saving} disabled={loading}><Save size={14} />保存来源</Button></div></div>
        <div className="mt-4 space-y-3">
          {!loading && !sources.some(source => !source.built_in) && <div className="rounded-lg border border-dashed border-[var(--border-soft)] p-4 text-sm text-[var(--text-secondary)]">尚未添加自定义来源，当前直接从 GitHub 获取更新。</div>}
          {sources.map((source, index) => !source.built_in && <div key={sourceKey(source, index)} className="grid min-w-0 gap-3 rounded-lg border border-[var(--border-soft)] bg-[var(--surface-muted)]/30 p-3 sm:grid-cols-2 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,2fr)_minmax(8rem,0.4fr)_auto_auto] xl:items-end">
            <div className="min-w-0"><Input label="名称" aria-label="名称" className="min-w-0" value={source.name} onChange={event => updateSource(index, { name: event.target.value })} placeholder="例如 下载镜像" /></div>
            <div className="min-w-0"><Input label="地址" aria-label="地址" className="min-w-0" value={source.base_url} onChange={event => updateSource(index, { base_url: event.target.value })} placeholder="https://..." type="url" /></div>
            <div className="min-w-0"><Input label="优先级" aria-label="优先级" className="min-w-0" type="number" min={0} max={99999} step={1} value={source.priority} onChange={event => updateSource(index, { priority: Number(event.target.value) })} /></div>
            <label className="flex h-11 items-center gap-2 px-1 text-sm text-[var(--text-secondary)]"><input type="checkbox" checked={source.enabled} onChange={event => updateSource(index, { enabled: event.target.checked })} />启用</label>
            <button type="button" title="移除更新源" aria-label="移除更新源" onClick={() => removeSource(index)} className="flex h-11 w-11 items-center justify-center rounded-lg text-[var(--text-secondary)] transition hover:bg-red-50 hover:text-red-600"><Trash2 size={16} /></button>
          </div>)}
          {sources.filter(source => source.built_in).map(source => <div key="built-in-github" className="flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-lg border border-[var(--border-soft)] bg-[var(--surface-muted)]/30 px-4 py-3">
            <div className="min-w-0"><div className="flex flex-wrap items-center gap-2 text-sm font-semibold">GitHub<Badge variant="default">默认来源</Badge></div><div className="mt-1 break-all text-xs text-[var(--text-secondary)]">{source.base_url}</div></div>
            <p className="text-xs text-[var(--text-secondary)]">自定义来源不可用时使用，地址固定</p>
          </div>)}
        </div>
      </Card>
    </div>
  );
};

export default SystemUpdate;
