import { request } from './request';

export interface UpdateSource {
  id?: number;
  name: string;
  base_url: string;
  enabled: boolean;
  priority: number;
  config_version?: number;
  built_in?: boolean;
}

export interface VersionInfo {
  version: string;
  build_time?: string;
  source_revision?: string;
}

export interface UpdateAsset {
  name: string;
  platform?: string;
  arch?: string;
  size: number;
  sha256: string;
  asset_path?: string;
}

export interface UpdateManifest {
  schema?: number;
  product?: string;
  channel?: string;
  version: string;
  tag?: string;
  commit?: string;
  build_time?: string;
  assets?: UpdateAsset[];
}

export interface UpdateCheckResult {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  source: string;
  manifest: UpdateManifest;
}

export interface UpdateDownloadResult {
  path?: string;
  source?: string;
  version?: string;
  asset?: UpdateAsset;
  size?: number;
  sha256?: string;
  state: 'idle' | 'downloading' | 'staged' | 'failed' | 'queued' | 'installing' | 'completed' | 'rolled_back' | 'recovery_required';
  error?: string;
  message?: string;
  install_available?: boolean;
  install_reason?: string;
}

export interface UpdateInstallPlan {
  version: string;
  plan: {
    digest: string;
    migrations: Array<{ Version: string; Name: string; Filename: string; Checksum: string; SQL: string }>;
  };
}

export type UpdateSourceInput = Omit<UpdateSource, 'id' | 'config_version'> & {
  id?: number;
  config_version?: number;
};

export interface UpdateSourceConfiguration {
  sources: UpdateSource[];
  revision: number;
}

export const fetchUpdateSources = (signal?: AbortSignal) =>
  request<UpdateSourceConfiguration>('/admin/system/update-sources', { signal });

export const saveUpdateSources = (sources: UpdateSourceInput[], expectedRevision: number, signal?: AbortSignal) =>
  request<UpdateSourceConfiguration>('/admin/system/update-sources', {
    method: 'PUT',
    body: JSON.stringify({ sources, expected_revision: expectedRevision }),
    signal,
  });

export const fetchSystemVersion = (signal?: AbortSignal) =>
  request<VersionInfo>('/admin/system/version', { signal });

export const checkSystemUpdate = (signal?: AbortSignal) =>
  request<UpdateCheckResult>('/admin/system/update/check', { signal });

export const downloadSystemUpdate = (signal?: AbortSignal) =>
  request<UpdateDownloadResult>('/admin/system/update/download', {
    method: 'POST',
    body: JSON.stringify({}),
    signal,
  });

export const fetchSystemUpdateStatus = (signal?: AbortSignal) =>
  request<UpdateDownloadResult>('/admin/system/update/status', { signal });

export const prepareSystemUpdate = (signal?: AbortSignal) =>
  request<UpdateInstallPlan>('/admin/system/update/prepare', { method: 'POST', body: '{}', signal });

export const installSystemUpdate = (version: string, planDigest: string, signal?: AbortSignal) =>
  request<UpdateDownloadResult>('/admin/system/update/install', {
    method: 'POST', body: JSON.stringify({ version, plan_digest: planDigest, confirmed: true }), signal,
  });
