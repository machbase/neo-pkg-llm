import type { AppConfig, ApiResponse } from '../types/settings';

// ── Config list ──
export async function getConfigList(): Promise<string[]> {
  const res = await fetch('/api/configs');
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const body = (await res.json()) as ApiResponse<{ configs: string[] }>;
  if (!body.success) throw new Error(body.reason);
  return body.data?.configs ?? [];
}

// ── Config detail ──
export async function getConfig(name: string): Promise<AppConfig> {
  const res = await fetch(`/api/configs/${encodeURIComponent(name)}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const body = (await res.json()) as ApiResponse<AppConfig>;
  if (!body.success) throw new Error(body.reason);
  return body.data as AppConfig;
}

// ── Config create ──
export async function createConfig(config: AppConfig): Promise<string> {
  const res = await fetch('/api/configs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
  const body = (await res.json()) as ApiResponse<{ name: string }>;
  if (!body.success) throw new Error(body.reason);
  return body.data?.name ?? '';
}

// ── Config update ──
export async function updateConfig(name: string, config: AppConfig): Promise<string> {
  const res = await fetch(`/api/configs/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
  const body = (await res.json()) as ApiResponse<{ name: string }>;
  if (!body.success) throw new Error(body.reason);
  return body.data?.name ?? '';
}

// ── Config delete ──
export async function deleteConfig(name: string): Promise<void> {
  const res = await fetch(`/api/configs/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
  const body = (await res.json()) as ApiResponse<{ name: string }>;
  if (!body.success) throw new Error(body.reason);
}

