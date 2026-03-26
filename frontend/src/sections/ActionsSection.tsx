import { useState } from "react";
import type { AppConfig, ToastType } from "../types/settings";
import { createConfig, updateConfig } from "../services/settingsApi";

interface Props {
    config: AppConfig;
    configName: string | null;
    showToast: (message: string, type: ToastType) => void;
    onSaved: (name: string) => void;
}

export function ActionsSection({ config, configName, showToast, onSaved }: Props) {
    const [saving, setSaving] = useState(false);

    const isNew = configName === null;

    const handleSave = async () => {
        setSaving(true);
        try {
            let savedName: string;
            if (isNew) {
                savedName = await createConfig(config);
                showToast(`Config "${savedName}" created.`, "success");
            } else {
                savedName = await updateConfig(configName, config);
                showToast(`Config "${savedName}" saved.`, "success");
            }
            onSaved(savedName);
        } catch (e) {
            showToast(`Save failed: ${e instanceof Error ? e.message : "unknown error"}`, "error");
        }
        setSaving(false);
    };

    return (
        <div className="panel-card">
            <div className="panel-card-head">
                <div>
                    <h3>Save &amp; Apply</h3>
                    <p>{isNew ? "Create a new configuration" : `Editing: ${configName}`}</p>
                </div>
            </div>

            <div className="action-bar">
                <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
                    {saving ? (
                        <span className="spinner" />
                    ) : (
                        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8">
                            <path d="M13.5 11.5v1a1 1 0 01-1 1h-9a1 1 0 01-1-1v-1M8 2v8M5 7l3 3 3-3" />
                        </svg>
                    )}
                    {isNew ? "Create Config" : "Save Settings"}
                </button>
            </div>
        </div>
    );
}
