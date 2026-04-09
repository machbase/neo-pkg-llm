interface Props {
    name: string
    className?: string
    style?: React.CSSProperties
}

export default function Icon({ name, className = '', style }: Props) {
    return (
        <span className={`material-symbols-outlined ${className}`} style={style}>
            {name}
        </span>
    )
}
