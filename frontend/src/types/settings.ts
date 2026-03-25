export interface ServerConfig {
    port: string;
}

export interface MachbaseConfig {
    host: string;
    port: string;
    user: string;
    password: string;
}

export interface ModelEntry {
    name: string;
    model_id: string;
}

export interface ProviderConfig {
    api_key: string;
    models: ModelEntry[];
}

export interface OllamaConfig {
    base_url: string;
    models: ModelEntry[];
}

export interface AppConfig {
    server: ServerConfig;
    machbase: MachbaseConfig;
    claude: ProviderConfig;
    chatgpt: ProviderConfig;
    gemini: ProviderConfig;
    ollama: OllamaConfig;
}

export type ApiProvider = "claude" | "chatgpt" | "gemini";
export type ModelProvider = "claude" | "chatgpt" | "gemini" | "ollama";

export type ToastType = "success" | "error" | "warning";

export interface ToastItem {
    id: string;
    message: string;
    type: ToastType;
}

export interface ApiResponse<T> {
    success: boolean;
    reason: string;
    elapse: string;
    data: T | null;
}

export function defaultConfig(): AppConfig {
    return {
        server: { port: "" },
        machbase: { host: "", port: "", user: "", password: "" },
        claude: { api_key: "", models: [] },
        chatgpt: { api_key: "", models: [] },
        gemini: { api_key: "", models: [] },
        ollama: { base_url: "", models: [] },
    };
}
