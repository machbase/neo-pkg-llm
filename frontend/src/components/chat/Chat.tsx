import { usePkgChat } from '../../hooks/usePkgChat';
import { ChatView } from './ChatView';

interface ChatProps {
    onOpenSettings?: () => void;
}

export const Chat = ({ onOpenSettings }: ChatProps) => {
    const chatLogic = usePkgChat();
    return <ChatView {...chatLogic} onOpenSettings={onOpenSettings} />;
};
