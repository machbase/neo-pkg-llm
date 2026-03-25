import { useState } from 'react';
import type { AppConfig, ToastType } from '../types/settings';
import { createConfig, updateConfig } from '../services/settingsApi';

interface Props {
  config: AppConfig;
  configName: string | null;
  showToast: (message: string, type: ToastType) => void;
  onSaved: (name: string) => void;
}

function maskApiKey(key: string): string {
  if (!key || key.length <= 8) return key;
  return key.slice(0, 8) + '...';
}

function buildPreview(config: AppConfig): AppConfig {
  return {
    ...config,
    claude:  { ...config.claude,  api_key: maskApiKey(config.claude.api_key) },
    chatgpt: { ...config.chatgpt, api_key: maskApiKey(config.chatgpt.api_key) },
    gemini:  { ...config.gemini,  api_key: maskApiKey(config.gemini.api_key) },
  };
}

function syntaxHighlight(json: string): string {
  return json.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
    (match) => {
      let cls = 'json-num';
      if (/^"/.test(match)) cls = /:$/.test(match) ? 'json-key' : 'json-str';
      else if (/true|false/.test(match)) cls = 'json-bool';
      else if (/null/.test(match)) cls = 'json-null';
      return `<span class="${cls}">${match}</span>`;
    },
  );
}

export function ActionsSection({ config, configName, showToast, onSaved }: Props) {
  const [previewOpen, setPreviewOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  const isNew = configName === null;

  const handleSave = async () => {
    setSaving(true);
    try {
      let savedName: string;
      if (isNew) {
        savedName = await createConfig(config);
        showToast(`Config "${savedName}" created.`, 'success');
      } else {
        savedName = await updateConfig(configName, config);
        showToast(`Config "${savedName}" saved.`, 'success');
      }
      onSaved(savedName);
    } catch (e) {
      showToast(`Save failed: ${e instanceof Error ? e.message : 'unknown error'}`, 'error');
    }
    setSaving(false);
  };

  const previewJson = syntaxHighlight(JSON.stringify(buildPreview(config), null, 2));

  return (
    <div className="panel-card">
      <div className="panel-card-head">
        <div>
          <h3>Save &amp; Apply</h3>
          <p>{isNew ? 'Create a new configuration' : `Editing: ${configName}`}</p>
        </div>
        <button className="btn-ghost" onClick={() => setPreviewOpen((v) => !v)}>
          {'{ }'} {previewOpen ? 'Hide Config' : 'Preview Config'}
        </button>
      </div>

      <div className={`json-preview${previewOpen ? ' open' : ''}`}>
        <pre className="json-pre" dangerouslySetInnerHTML={{ __html: previewJson }} />
      </div>

      <div className="action-bar">
        <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
          {saving ? (
            <span className="spinner" />
          ) : (
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8">
              <path d="M13.5 11.5v1a1 1 0 01-1 1h-9a1 1 0 01-1-1v-1M8 2v8M5 7l3 3 3-3"/>
            </svg>
          )}
          {isNew ? 'Create Config' : 'Save Settings'}
        </button>
      </div>
    </div>
  );
}
