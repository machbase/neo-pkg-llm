import { useCallback, useEffect, useRef, useState } from 'react';
import { Sidebar } from './components/Sidebar';
import { ServerSection } from './sections/ServerSection';
import { MachbaseSection } from './sections/MachbaseSection';
import { ApiKeysSection } from './sections/ApiKeysSection';
import { ModelsSection } from './sections/ModelsSection';
import { ActionsSection } from './sections/ActionsSection';
import { getConfigList, getConfig, deleteConfig } from './services/settingsApi';
import { defaultConfig } from './types/settings';
import { getCurrentUser, isSysUser } from './utils/auth';
import type { AppConfig, ModelProvider, ToastItem, ToastType } from './types/settings';

export function App() {
  const [configs, setConfigs] = useState<string[]>([]);
  const [selectedConfig, setSelectedConfig] = useState<string | null>(null);
  const [config, setConfig] = useState<AppConfig>(defaultConfig());
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [refreshing, setRefreshing] = useState(false);
  const toastTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const showToast = useCallback((message: string, type: ToastType) => {
    const id = Math.random().toString(36).slice(2) + Date.now().toString(36);
    setToasts((prev) => [...prev, { id, message, type }]);
    const timer = setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
      toastTimers.current.delete(id);
    }, 3000);
    toastTimers.current.set(id, timer);
  }, []);

  const loadConfigList = useCallback(async () => {
    try {
      const all = await getConfigList();
      const user = getCurrentUser();
      // sys user sees everything; other users see only their own config
      const list = isSysUser() || !user
        ? all
        : all.filter((name) => name === user);
      setConfigs(list);
      return list;
    } catch {
      showToast('Failed to load config list.', 'error');
      return [];
    }
  }, [showToast]);

  const loadConfig = useCallback(async (name: string) => {
    try {
      const data = await getConfig(name);
      setConfig(data);
      setSelectedConfig(name);
    } catch {
      showToast(`Failed to load config "${name}".`, 'error');
    }
  }, [showToast]);

  useEffect(() => {
    (async () => {
      const list = await loadConfigList();
      if (list.length > 0) {
        await loadConfig(list[0]);
      }
    })();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    await loadConfigList();
    setRefreshing(false);
  }, [loadConfigList]);

  const handleSelectConfig = useCallback((name: string) => {
    loadConfig(name);
  }, [loadConfig]);

  const handleNewConfig = useCallback(() => {
    setSelectedConfig(null);
    setConfig(defaultConfig());
  }, []);

  const handleDelete = useCallback(async (name: string) => {
    try {
      await deleteConfig(name);
      showToast(`Config "${name}" deleted.`, 'success');
      const list = await loadConfigList();
      if (selectedConfig === name) {
        if (list.length > 0) {
          await loadConfig(list[0]);
        } else {
          setSelectedConfig(null);
          setConfig(defaultConfig());
        }
      }
    } catch (e) {
      showToast(`Delete failed: ${e instanceof Error ? e.message : 'unknown error'}`, 'error');
    }
  }, [showToast, loadConfigList, loadConfig, selectedConfig]);

  const handleSaved = useCallback(async (name: string) => {
    await loadConfigList();
    setSelectedConfig(name);
  }, [loadConfigList]);

  const handleServerChange = useCallback((server: AppConfig['server']) => {
    setConfig((prev) => ({ ...prev, server }));
  }, []);

  const handleMachbaseChange = useCallback((machbase: AppConfig['machbase']) => {
    setConfig((prev) => ({ ...prev, machbase }));
  }, []);

  const handleApiKeyChange = useCallback((provider: 'claude' | 'chatgpt' | 'gemini', key: string) => {
    setConfig((prev) => ({
      ...prev,
      [provider]: { ...prev[provider], api_key: key },
    }));
  }, []);

  const handleOllamaUrlChange = useCallback((url: string) => {
    setConfig((prev) => ({ ...prev, ollama: { ...prev.ollama, base_url: url } }));
  }, []);

  const handleModelsChange = useCallback((provider: ModelProvider, models: AppConfig['claude']['models']) => {
    setConfig((prev) => ({
      ...prev,
      [provider]: { ...prev[provider], models },
    }));
  }, []);

  return (
    <div className="settings-shell">
      <Sidebar
        configs={configs}
        selectedConfig={selectedConfig}
        onSelectConfig={handleSelectConfig}
        onNewConfig={handleNewConfig}
        onRefresh={handleRefresh}
        onDelete={handleDelete}
        loading={refreshing}
        refreshing={refreshing}
      />

      <main className="content-shell">
        <section className="content-area">
          <header className="content-header">
            <h1>{selectedConfig === null ? 'New Configuration' : `Configuration: ${selectedConfig}`}</h1>
            <p>Manage LLM providers, API keys, models, and connection settings.</p>
          </header>

          <div className="sections-stack">
            <ServerSection config={config.server} onChange={handleServerChange} />
            <MachbaseSection config={config.machbase} onChange={handleMachbaseChange} />
            <ApiKeysSection
              claude={config.claude}
              chatgpt={config.chatgpt}
              gemini={config.gemini}
              ollama={config.ollama}
              onKeyChange={handleApiKeyChange}
              onOllamaUrlChange={handleOllamaUrlChange}
              showToast={showToast}
            />
            <ModelsSection
              claude={config.claude}
              chatgpt={config.chatgpt}
              gemini={config.gemini}
              ollama={config.ollama}
              onChange={handleModelsChange}
            />
            <ActionsSection
              config={config}
              configName={selectedConfig}
              showToast={showToast}
              onSaved={handleSaved}
            />
          </div>
        </section>
      </main>

      {toasts.length > 0 && (
        <div className="toast-container" role="status" aria-live="polite">
          {toasts.map((t) => (
            <div key={t.id} className={`toast toast-${t.type}`}>
              <span className="toast-icon">{t.type === 'success' ? '✓' : t.type === 'error' ? '✕' : '⚠'}</span>
              <span>{t.message}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default App;
