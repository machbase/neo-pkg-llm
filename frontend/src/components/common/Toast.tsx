import { useApp } from '../../context/AppContext'
import Icon from './Icon'

const typeClass: Record<string, string> = {
    success: 'toast-success',
    error: 'toast-error',
    info: '',
}

const typeIcons: Record<string, string> = {
    success: 'check_circle',
    error: 'error',
    info: 'info',
}

export default function Toast() {
    const { notifications, dismissNotification } = useApp()

    if (notifications.length === 0) return null

    return (
        <div className="fixed bottom-4 right-4 z-[1000] space-y-2">
            {notifications.map(n => (
                <div
                    key={n.id}
                    className={`toast animate-slide-in ${typeClass[n.type] || ''}`}
                >
                    <Icon name={typeIcons[n.type] || typeIcons.info} />
                    <span className="flex-1">{n.message}</span>
                    <button onClick={() => dismissNotification(n.id)} className="opacity-60 hover:opacity-100 tooltip" data-tooltip="Close">
                        <Icon name="close" className="icon-sm" />
                    </button>
                </div>
            ))}
        </div>
    )
}
