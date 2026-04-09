import { useState } from 'react'
import { useApp } from '../context/AppContext'
import Icon from '../components/common/Icon'
import type { AppConfig } from '../types/settings'
import { createConfig, updateConfig } from '../services/settingsApi'

interface Props {
    config: AppConfig
    configName: string | null
    onSaved: (name: string) => void
}

export function ActionsSection({ config, configName, onSaved }: Props) {
    const { notify } = useApp()
    const [saving, setSaving] = useState(false)
    const isNew = configName === null

    const handleSave = async () => {
        setSaving(true)
        try {
            let savedName: string
            if (isNew) {
                savedName = await createConfig(config)
                notify(`Config "${savedName}" created.`, 'success')
            } else {
                savedName = await updateConfig(configName, config)
                notify(`Config "${savedName}" saved.`, 'success')
            }
            onSaved(savedName)
        } catch (e) {
            notify(`Save failed: ${e instanceof Error ? e.message : 'unknown error'}`, 'error')
        }
        setSaving(false)
    }

    return (
        <div className="card">
            <div className="card-title">
                <div>
                    <h3>Save & Apply</h3>
                    <p className="text-sm text-on-surface-secondary mt-1">{isNew ? 'Create a new configuration' : `Editing: ${configName}`}</p>
                </div>
            </div>

            <div className="flex items-center gap-3">
                <button className="btn btn-content btn-primary" onClick={handleSave} disabled={saving}>
                    {saving
                        ? <span className="spinner" />
                        : <Icon name="save" className="icon-sm" />
                    }
                    {isNew ? 'Create Config' : 'Save Settings'}
                </button>
            </div>
        </div>
    )
}
