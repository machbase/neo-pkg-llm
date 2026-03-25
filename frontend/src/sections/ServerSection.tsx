import type { ServerConfig } from '../types/settings';

interface Props {
  config: ServerConfig;
  onChange: (config: ServerConfig) => void;
}

export function ServerSection({ config, onChange }: Props) {
  return (
    <div className="panel-card">
      <div className="panel-card-head">
        <div>
          <h3>Server</h3>
          <p>Application server settings</p>
        </div>
      </div>

      <div className="form-grid">
        <div className="field-row">
          <label htmlFor="srv-port">Port</label>
          <input
            id="srv-port"
            type="text"
            placeholder="8884"
            value={config.port}
            onChange={(e) => onChange({ ...config, port: e.target.value })}
          />
        </div>
      </div>
    </div>
  );
}
