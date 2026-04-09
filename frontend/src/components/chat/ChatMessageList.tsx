import type { Message, UserMessageAlign } from '../../types/chat';
import { ChatMessageItem } from './ChatMessageItem';
import { LoadingDots } from './LoadingDots';

interface ChatMessageListProps {
    messages: Message[];
    isProcessingAnswer?: boolean;
    userMessageAlign?: UserMessageAlign;
    scrollRef?: React.RefObject<HTMLDivElement | null>;
}

export const ChatMessageList = ({
    messages,
    isProcessingAnswer = false,
    userMessageAlign = 'left',
    scrollRef,
}: ChatMessageListProps) => (
    <div ref={scrollRef} className="chat-messages">
        {messages.map((message) => (
            <div className="chat-message-item-wrap" key={message.id}>
                <ChatMessageItem message={message} userMessageAlign={userMessageAlign} />
            </div>
        ))}
        {isProcessingAnswer && (
            <div className="chat-loading-wrap">
                <LoadingDots />
            </div>
        )}
        <div className="chat-messages-anchor" />
    </div>
);
