import type { ApiProvider, ProviderConfig, OllamaConfig, ToastType } from '../types/settings';

type AnyProvider = ApiProvider | 'ollama';

interface Props {
  claude:  ProviderConfig;
  chatgpt: ProviderConfig;
  gemini:  ProviderConfig;
  ollama:  OllamaConfig;
  onKeyChange: (provider: ApiProvider, key: string) => void;
  onOllamaUrlChange: (url: string) => void;
  showToast: (message: string, type: ToastType) => void;
}

const PROVIDER_META: Record<AnyProvider, { label: string; placeholder: string; fieldLabel: string }> = {
  claude:  { label: 'Claude',  placeholder: 'sk-ant-api03-...', fieldLabel: 'API Key' },
  chatgpt: { label: 'ChatGPT', placeholder: 'sk-proj-...',      fieldLabel: 'API Key' },
  gemini:  { label: 'Gemini',  placeholder: 'AIzaSy...',        fieldLabel: 'API Key' },
  ollama:  { label: 'Ollama',  placeholder: 'http://localhost:11434', fieldLabel: 'Base URL' },
};

const ALL_PROVIDERS: AnyProvider[] = ['claude', 'chatgpt', 'gemini', 'ollama'];

export function ApiKeysSection({
  claude, chatgpt, gemini, ollama,
  onKeyChange, onOllamaUrlChange,
}: Props) {
  const getValue = (p: AnyProvider): string => {
    if (p === 'ollama') return ollama.base_url;
    return { claude, chatgpt, gemini }[p].api_key;
  };

  const handleChange = (p: AnyProvider, value: string) => {
    if (p === 'ollama') {
      onOllamaUrlChange(value);
    } else {
      onKeyChange(p, value);
    }
  };

  return (
    <div className="panel-card">
      <div className="panel-card-head">
        <div>
          <h3>API Keys &amp; Endpoints</h3>
          <p>LLM provider authentication keys and endpoints</p>
        </div>
      </div>

      <table className="data-table">
        <thead>
          <tr>
            <th style={{ width: 110 }}>Provider</th>
            <th>Value</th>
          </tr>
        </thead>
        <tbody>
          {ALL_PROVIDERS.map((p) => (
            <tr key={p}>
              <td><span className={`badge badge-${p}`}>{PROVIDER_META[p].label}</span></td>
              <td>
                <input
                  type={p === 'ollama' ? 'text' : 'password'}
                  placeholder={PROVIDER_META[p].placeholder}
                  value={getValue(p)}
                  onChange={(e) => handleChange(p, e.target.value)}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
