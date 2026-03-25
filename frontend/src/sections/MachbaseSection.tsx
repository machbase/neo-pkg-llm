import type { MachbaseConfig } from '../types/settings';

interface Props {
  config: MachbaseConfig;
  onChange: (config: MachbaseConfig) => void;
}

export function MachbaseSection({ config, onChange }: Props) {
  const set = (field: keyof MachbaseConfig) => (e: React.ChangeEvent<HTMLInputElement>) =>
    onChange({ ...config, [field]: e.target.value });

  return (
    <div className="panel-card">
      <div className="panel-card-head">
        <div>
          <h3>Machbase Connection</h3>
          <p>Database connection settings for Machbase Neo</p>
        </div>
      </div>

      <div className="form-grid">
        <div className="field-row">
          <label htmlFor="mb-host">Host</label>
          <input id="mb-host" type="text" placeholder="127.0.0.1" value={config.host} onChange={set('host')} />
        </div>
        <div className="field-row">
          <label htmlFor="mb-port">Port</label>
          <input id="mb-port" type="text" placeholder="5654" value={config.port} onChange={set('port')} />
        </div>
        <div className="field-row">
          <label htmlFor="mb-user">User ID</label>
          <input id="mb-user" type="text" placeholder="sys" value={config.user} onChange={set('user')} />
        </div>
        <div className="field-row">
          <label htmlFor="mb-work-dir">Work Directory</label>
          <input id="mb-work-dir" type="text" placeholder="C:/path/to/machbase-neo" value={config.work_dir} onChange={set('work_dir')} />
        </div>
      </div>
    </div>
  );
}
