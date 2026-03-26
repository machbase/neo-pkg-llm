import type { AppConfig, ApiResponse } from '../types/settings';
import { defaultConfig } from '../types/settings';

// ── Config list ──
export async function getConfigList(): Promise<string[]> {
  const res = await fetch('/api/configs');
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const body = (await res.json()) as ApiResponse<{ configs: Array<string | { name: string; running: boolean }> }>;
  if (!body.success) throw new Error(body.reason);
  const raw = body.data?.configs ?? [];
  return raw.map((item) => (typeof item === 'string' ? item : item.name));
}

// ── Config detail ──
export async function getConfig(name: string): Promise<AppConfig> {
  const res = await fetch(`/api/configs/${encodeURIComponent(name)}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const body = (await res.json()) as ApiResponse<{ config: AppConfig; running: boolean }>;
  if (!body.success) throw new Error(body.reason);
  const defaults = defaultConfig();
  const data = body.data?.config ?? ({} as Partial<AppConfig>);
  return {
    server: { ...defaults.server, ...data.server },
    machbase: { ...defaults.machbase, ...data.machbase },
    claude: { ...defaults.claude, ...data.claude },
    chatgpt: { ...defaults.chatgpt, ...data.chatgpt },
    gemini: { ...defaults.gemini, ...data.gemini },
    ollama: { ...defaults.ollama, ...data.ollama },
  };
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

