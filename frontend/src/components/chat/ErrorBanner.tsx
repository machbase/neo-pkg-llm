import Icon from '../common/Icon';

interface ErrorBannerProps {
    message: string;
    code?: number;
}

export const ErrorBanner = ({ code, message }: ErrorBannerProps) => {
    if (!message || message.length === 0) return null;

    return (
        <div className="chat-error-banner">
            <Icon name="error" className="chat-error-banner-icon" />
            <div className="chat-error-banner-body">
                {code && <div className="chat-error-banner-title">Error {code}</div>}
                <div className="chat-error-banner-message">{message}</div>
            </div>
        </div>
    );
};
