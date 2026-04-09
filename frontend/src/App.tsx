import { useCallback, useEffect, useState } from "react";
import { useApp } from "./context/AppContext";
import { getConfigList, getConfig, deleteConfig, createConfig, updateConfig } from "./services/settingsApi";
import { defaultConfig } from "./types/settings";
import { getCurrentUser } from "./utils/auth";
import { MachbaseSection } from "./sections/MachbaseSection";
import { ApiKeysSection } from "./sections/ApiKeysSection";
import { ModelsSection } from "./sections/ModelsSection";
import { Chat } from "./components/chat/Chat";
import Icon from "./components/common/Icon";
import ConfirmDialog from "./components/common/ConfirmDialog";
import Toast from "./components/common/Toast";
import type { AppConfig, ModelProvider } from "./types/settings";

type AppTab = "settings" | "chat" | null;

export default function App() {
    const { selectedConfig, setSelectedConfig, notify } = useApp();
    const [activeTab, setActiveTab] = useState<AppTab | null>(null);
    const [config, setConfig] = useState<AppConfig>(defaultConfig());
    const [saving, setSaving] = useState(false);
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

    const loadConfigList = useCallback(async () => {
        try {
            const all = await getConfigList();
            const user = getCurrentUser();
            return !user ? all : all.filter((name) => name === user);
        } catch {
            notify("Failed to load config list.", "error");
            return [];
        }
    }, [notify]);

    const loadConfig = useCallback(
        async (name: string) => {
            try {
                const data = await getConfig(name);
                setConfig(data);
                setSelectedConfig(name);
            } catch {
                notify(`Failed to load config "${name}".`, "error");
            }
        },
        [notify, setSelectedConfig]
    );

    const currentUser = getCurrentUser() ?? "sys";

    useEffect(() => {
        (async () => {
            const list = await loadConfigList();
            if (list.includes(currentUser)) {
                await loadConfig(currentUser);
                setActiveTab("chat");
            } else {
                setConfig((prev) => ({
                    ...prev,
                    machbase: {
                        ...prev.machbase,
                        host: "127.0.0.1",
                        port: "5656",
                        user: currentUser,
                    },
                }));
                setActiveTab("settings");
            }
        })();
    }, []); // eslint-disable-line react-hooks/exhaustive-deps

    const handleDelete = useCallback(async () => {
        if (!selectedConfig) return;
        try {
            await deleteConfig(selectedConfig);
            notify(`Config "${selectedConfig}" deleted.`, "success");
            const list = await loadConfigList();
            if (list.length > 0) {
                await loadConfig(list[0]);
            } else {
                setSelectedConfig(null);
                setConfig(defaultConfig());
            }
        } catch (e) {
            notify(`Delete failed: ${e instanceof Error ? e.message : "unknown error"}`, "error");
        }
        setShowDeleteConfirm(false);
    }, [notify, loadConfigList, loadConfig, selectedConfig, setSelectedConfig]);

    const handleSave = useCallback(async () => {
        setSaving(true);
        try {
            const isNew = selectedConfig === null;
            let savedName: string;
            if (isNew) {
                savedName = await createConfig(config);
                notify(`Config "${savedName}" created.`, "success");
            } else {
                savedName = await updateConfig(selectedConfig, config);
                notify(`Config "${savedName}" saved.`, "success");
            }
            await loadConfigList();
            setSelectedConfig(savedName);
            setActiveTab("chat");
        } catch (e) {
            notify(`Save failed: ${e instanceof Error ? e.message : "unknown error"}`, "error");
        }
        setSaving(false);
    }, [config, selectedConfig, notify, loadConfigList, setSelectedConfig]);

    const handleMachbaseChange = useCallback((machbase: AppConfig["machbase"]) => {
        setConfig((prev) => ({ ...prev, machbase }));
    }, []);

    const handleApiKeyChange = useCallback((provider: "claude" | "chatgpt" | "gemini", key: string) => {
        setConfig((prev) => ({
            ...prev,
            [provider]: { ...prev[provider], api_key: key },
        }));
    }, []);

    const handleOllamaUrlChange = useCallback((url: string) => {
        setConfig((prev) => ({ ...prev, ollama: { ...prev.ollama, base_url: url } }));
    }, []);

    const handleModelsChange = useCallback((provider: ModelProvider, models: AppConfig["claude"]["models"]) => {
        setConfig((prev) => ({
            ...prev,
            [provider]: { ...prev[provider], models },
        }));
    }, []);

    return (
        <>
            <div className="page bg-surface-alt" style={{ display: "flex", flexDirection: "column", height: "100vh" }}>
                {activeTab === null ? (
                    <div className="flex-1 flex items-center justify-center">
                        <span className="spinner" />
                    </div>
                ) : activeTab === "settings" ? (
                    <div className="flex-1 flex flex-col overflow-hidden">
                        <div className="page-header">
                            <div className="page-header-inner">
                                <div>
                                    <h1 className="page-title">{selectedConfig === null ? "New Configuration" : `Configuration: ${selectedConfig}`}</h1>
                                    <p className="page-desc">Manage LLM providers, API keys, models, and connection settings.</p>
                                </div>
                                <div className="flex items-center gap-2">
                                    <button className="btn btn-content btn-success" onClick={() => setActiveTab("chat")}>
                                        <Icon name="chat" className="icon-sm" /> Chat
                                    </button>
                                    {selectedConfig !== null && (
                                        <button className="btn btn-content btn-danger" onClick={() => setShowDeleteConfirm(true)}>
                                            <Icon name="delete" className="icon-sm" /> Delete
                                        </button>
                                    )}
                                    <button className="btn btn-content btn-primary" onClick={handleSave} disabled={saving}>
                                        {saving ? <span className="spinner" /> : <Icon name="save" className="icon-sm" />}
                                        {selectedConfig === null ? "Create Config" : "Save Settings"}
                                    </button>
                                </div>
                            </div>
                        </div>
                        <div className="page-body">
                            <div className="page-body-inner">
                                <div className="flex flex-col gap-4">
                                    <MachbaseSection config={config.machbase} onChange={handleMachbaseChange} />
                                    <ApiKeysSection
                                        claude={config.claude}
                                        chatgpt={config.chatgpt}
                                        gemini={config.gemini}
                                        ollama={config.ollama}
                                        onKeyChange={handleApiKeyChange}
                                        onOllamaUrlChange={handleOllamaUrlChange}
                                    />
                                    <ModelsSection claude={config.claude} chatgpt={config.chatgpt} gemini={config.gemini} ollama={config.ollama} onChange={handleModelsChange} />
                                </div>
                            </div>
                        </div>
                    </div>
                ) : (
                    <div className="flex-1 overflow-hidden">
                        <Chat onOpenSettings={() => setActiveTab("settings")} />
                    </div>
                )}
            </div>

            {showDeleteConfirm && selectedConfig && (
                <ConfirmDialog
                    title="Delete Configuration"
                    message={`Are you sure you want to delete "${selectedConfig}"? This action cannot be undone.`}
                    onConfirm={handleDelete}
                    onCancel={() => setShowDeleteConfirm(false)}
                />
            )}

            <Toast />
        </>
    );
}
