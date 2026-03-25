import { useEffect, useState } from "react";

interface Props {
    configs: string[];
    selectedConfig: string | null;
    onSelectConfig: (name: string) => void;
    onNewConfig: () => void;
    onRefresh: () => void;
    onDelete: (name: string) => Promise<void>;
    loading: boolean;
    refreshing: boolean;
}

export function Sidebar({ configs, selectedConfig, onSelectConfig, onNewConfig, onRefresh, onDelete, loading, refreshing }: Props) {
    const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
    const [deleting, setDeleting] = useState(false);

    useEffect(() => {
        if (!confirmTarget) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape" && !deleting) setConfirmTarget(null);
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [confirmTarget, deleting]);

    const handleConfirmDelete = async () => {
        if (!confirmTarget) return;
        setDeleting(true);
        try {
            await onDelete(confirmTarget);
        } finally {
            setDeleting(false);
            setConfirmTarget(null);
        }
    };

    return (
        <aside className="sidebar-panel" aria-label="Settings navigation">
            <div className="brand-block">
                <div className="brand-icon" aria-hidden="true">
                    <span />
                    <span />
                    <span />
                    <span />
                </div>
                <div>
                    <p className="brand-title">Machbase Neo AI</p>
                </div>
            </div>

            <nav className="sidebar-nav">
                <div className="sidebar-section-header">
                    <span className="sidebar-section-label">Configurations</span>
                    <button className="sidebar-refresh-btn" onClick={onRefresh} disabled={refreshing} title="Refresh list">
                        {refreshing ? <span className="spinner spinner-sm" /> : "↻"}
                    </button>
                </div>
                {loading ? (
                    <div className="sidebar-loading">
                        <span className="spinner" />
                    </div>
                ) : configs.length === 0 ? (
                    <div className="sidebar-empty">No configs</div>
                ) : (
                    configs.map((name) => (
                        <div
                            key={name}
                            className={`sidebar-tab${selectedConfig === name ? " is-active" : ""}`}
                            onClick={() => onSelectConfig(name)}
                            role="button"
                            tabIndex={0}
                            onKeyDown={(e) => {
                                if (e.key === "Enter" || e.key === " ") onSelectConfig(name);
                            }}
                        >
                            <span className="tab-dot" aria-hidden="true" />
                            <span className="sidebar-tab-name">{name}</span>
                            <button
                                className="sidebar-tab-delete"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    setConfirmTarget(name);
                                }}
                                title={`Delete ${name}`}
                            >
                                <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                                    <path d="M2 4h12M5 4V3a1 1 0 011-1h4a1 1 0 011 1v1M4 4v9.5a1 1 0 001 1h6a1 1 0 001-1V4M6.5 7v4M9.5 7v4" />
                                </svg>
                            </button>
                        </div>
                    ))
                )}
                <button className="sidebar-tab sidebar-add-btn" onClick={onNewConfig}>
                    <span className="tab-plus" aria-hidden="true">
                        +
                    </span>
                    <span>New Config</span>
                </button>
            </nav>

            {confirmTarget !== null && (
                <div className="modal-overlay" onClick={() => !deleting && setConfirmTarget(null)}>
                    <div className="modal-box" onClick={(e) => e.stopPropagation()}>
                        <p className="modal-title">Delete Configuration</p>
                        <p className="modal-body">
                            Are you sure you want to delete <strong>{confirmTarget}</strong>?
                        </p>
                        <div className="modal-actions">
                            <button className="btn-ghost" onClick={() => setConfirmTarget(null)} disabled={deleting}>
                                Cancel
                            </button>
                            <button className="btn btn-danger-fill" onClick={handleConfirmDelete} disabled={deleting}>
                                {deleting ? <span className="spinner spinner-sm" /> : "Delete"}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </aside>
    );
}
