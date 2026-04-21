import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkBreaks from 'remark-breaks'
import { cn } from '@/lib/utils'

// Chat-scoped markdown renderer. Small vertical rhythm, native tailwind tokens,
// opens links in a new tab. No raw HTML.
export function Markdown({ children, className }: { children: string; className?: string }) {
  return (
    <div className={cn('text-sm text-foreground [&>*:first-child]:mt-0 [&>*:last-child]:mb-0', className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkBreaks]}
        components={{
          p: ({ node, ...props }) => <p className="mb-2" {...props} />,
          ul: ({ node, ...props }) => <ul className="mb-2 list-disc space-y-1 pl-5" {...props} />,
          ol: ({ node, ...props }) => <ol className="mb-2 list-decimal space-y-1 pl-5" {...props} />,
          li: ({ node, ...props }) => <li className="leading-snug" {...props} />,
          a: ({ node, ...props }) => (
            <a
              className="text-primary underline hover:no-underline"
              target="_blank"
              rel="noreferrer"
              {...props}
            />
          ),
          strong: ({ node, ...props }) => <strong className="font-semibold" {...props} />,
          em: ({ node, ...props }) => <em className="italic" {...props} />,
          code: ({ node, className, children, ...props }) => {
            const isBlock = /language-/.test(className || '')
            if (isBlock) {
              return (
                <code className="font-mono text-xs" {...props}>
                  {children}
                </code>
              )
            }
            return (
              <code
                className="rounded bg-muted/60 px-1 py-0.5 font-mono text-[0.85em]"
                {...props}
              >
                {children}
              </code>
            )
          },
          pre: ({ node, ...props }) => (
            <pre
              className="my-2 overflow-x-auto rounded-md border border-border bg-muted/30 p-2 font-mono text-xs"
              {...props}
            />
          ),
          blockquote: ({ node, ...props }) => (
            <blockquote
              className="my-2 border-l-2 border-border pl-3 italic text-muted-foreground"
              {...props}
            />
          ),
          h1: ({ node, ...props }) => <h3 className="mb-1 mt-3 text-base font-semibold" {...props} />,
          h2: ({ node, ...props }) => <h3 className="mb-1 mt-3 text-base font-semibold" {...props} />,
          h3: ({ node, ...props }) => <h3 className="mb-1 mt-3 text-sm font-semibold" {...props} />,
          h4: ({ node, ...props }) => <h4 className="mb-1 mt-2 text-sm font-semibold" {...props} />,
          hr: () => <hr className="my-3 border-border" />,
          table: ({ node, ...props }) => (
            <div className="my-2 overflow-x-auto">
              <table className="w-full border-collapse text-xs" {...props} />
            </div>
          ),
          th: ({ node, ...props }) => (
            <th className="border-b border-border px-2 py-1 text-left font-medium text-muted-foreground" {...props} />
          ),
          td: ({ node, ...props }) => (
            <td className="border-b border-border/40 px-2 py-1" {...props} />
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
