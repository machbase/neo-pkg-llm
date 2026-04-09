export interface Message {
    id: string;
    content: string;
    timestamp: number;
    role: 'user' | 'assistant';
    type: 'block' | 'msg' | 'answer' | 'question' | 'error';
    isProcess: boolean;
    isInterrupt: boolean;
}

export interface PkgModel {
    name: string;
    model_id?: string;
}

export interface PkgProvider {
    provider: string;
    models: PkgModel[];
}

export interface PkgSelectedModel {
    provider: string;
    model: string;
    name: string;
}

export type UserMessageAlign = 'left' | 'right';
