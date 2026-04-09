import { useEffect, useMemo, useRef } from 'react';
import { Marked } from 'marked';
import { markedHighlight } from 'marked-highlight';
import hljs from 'highlight.js';

const marked = new Marked(
    markedHighlight({
        langPrefix: 'hljs language-',
        highlight(code: string, lang: string) {
            if (lang && hljs.getLanguage(lang)) {
                return hljs.highlight(code, { language: lang }).value;
            }
            return hljs.highlightAuto(code).value;
        },
    }),
    {
        renderer: {
            link({ href, text }) {
                const escaped = href.replace(/"/g, '&quot;');
                return `<a href="${escaped}" onclick="event.preventDefault();(window.top||window).open('${escaped}','_blank')" rel="noopener noreferrer">${text}</a>`;
            },
        },
    }
);

interface RenderMdProps {
    content: string;
    isInterrupt: boolean;
}

export const RenderMd = ({ content, isInterrupt }: RenderMdProps) => {
    const containerRef = useRef<HTMLDivElement>(null);

    const html = useMemo(() => {
        if (isInterrupt || !content) return '';
        return marked.parse(content) as string;
    }, [content, isInterrupt]);

    const COPY_ICON = `<svg viewBox="0 0 24 24" fill="rgba(255,255,255,0.5)" height="100%" width="100%">
        <path d="M20 2H10c-1.103 0-2 .897-2 2v4H4c-1.103 0-2 .897-2 2v10c0 1.103.897 2 2 2h10c1.103 0 2-.897 2-2v-4h4c1.103 0 2-.897 2-2V4c0-1.103-.897-2-2-2zM4 20V10h10l.002 10H4zm16-6h-4v-4c0-1.103-.897-2-2-2h-4V4h10v10z"/>
    </svg>`;

    useEffect(() => {
        const container = containerRef.current;
        if (!container || !html) return;

        container.querySelectorAll('pre code').forEach((codeElement) => {
            const preElement = codeElement.parentElement as HTMLPreElement;
            if (!preElement || preElement.querySelector('.code-copy-button')) return;

            const copyButton = document.createElement('button');
            copyButton.className = 'code-copy-button';
            copyButton.innerHTML = COPY_ICON;

            copyButton.addEventListener('click', (e) => {
                e.stopPropagation();
                const cloned = codeElement.cloneNode(true) as HTMLElement;
                cloned.querySelectorAll('span[style*="user-select:none"]').forEach((el) => el.remove());
                const code = cloned.textContent || '';

                const showSuccess = () => {
                    copyButton.classList.add('copied');
                    copyButton.innerHTML = `<svg viewBox="0 0 24 24" fill="#71e071" height="100%" width="100%"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>`;
                    setTimeout(() => { copyButton.classList.remove('copied'); copyButton.innerHTML = COPY_ICON; }, 2000);
                };

                const showError = () => {
                    copyButton.innerHTML = `<svg viewBox="0 0 24 24" fill="#ff5353" height="100%" width="100%"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>`;
                    setTimeout(() => { copyButton.innerHTML = COPY_ICON; }, 2000);
                };

                try {
                    if (navigator.clipboard && window.isSecureContext) {
                        navigator.clipboard.writeText(code).then(showSuccess, showError);
                    } else {
                        // Fallback for non-secure contexts
                        const textarea = document.createElement('textarea');
                        textarea.value = code;
                        textarea.style.position = 'fixed';
                        textarea.style.opacity = '0';
                        document.body.appendChild(textarea);
                        textarea.select();
                        document.execCommand('copy');
                        document.body.removeChild(textarea);
                        showSuccess();
                    }
                } catch {
                    showError();
                }
            });

            preElement.appendChild(copyButton);
        });
    });

    return (
        <div
            ref={containerRef}
            className="chat-markdown markdown-body"
            style={{ display: 'flex', flexDirection: 'column', width: '100%' }}
            dangerouslySetInnerHTML={{ __html: html }}
        />
    );
};
