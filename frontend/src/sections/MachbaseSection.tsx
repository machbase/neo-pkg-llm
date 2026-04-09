import type { MachbaseConfig } from '../types/settings'

interface Props {
    config: MachbaseConfig
    onChange: (config: MachbaseConfig) => void
}

export function MachbaseSection({ config, onChange }: Props) {
    const set = (field: keyof MachbaseConfig) => (e: React.ChangeEvent<HTMLInputElement>) =>
        onChange({ ...config, [field]: e.target.value })

    return (
        <div className="card">
            <div className="card-title">
                <div>
                    <h3>Machbase Connection</h3>
                    <p className="text-sm text-on-surface-secondary mt-1">Database connection settings for Machbase Neo</p>
                </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
                <label className="form-label">
                    <span>Host</span>
                    <input type="text" placeholder="127.0.0.1" value={config.host} onChange={set('host')} />
                </label>
                <label className="form-label">
                    <span>Port</span>
                    <input type="text" placeholder="5654" value={config.port} onChange={set('port')} />
                </label>
                <label className="form-label">
                    <span>User ID</span>
                    <input type="text" placeholder="sys" value={config.user} onChange={set('user')} />
                </label>
                <label className="form-label">
                    <span>Password</span>
                    <input type="password" placeholder="••••••••" value={config.password} onChange={set('password')} />
                </label>
            </div>
        </div>
    )
}
