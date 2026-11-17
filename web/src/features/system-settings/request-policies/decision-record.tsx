import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'

import type { PolicyEvent } from './api'
import { policyLabel } from './policy-label'

function formatUpstreamErrorText(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) {
    return ''
  }
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (parsed !== null && typeof parsed === 'object') {
        return JSON.stringify(parsed, null, 2)
      }
    } catch {
      return trimmed
    }
  }
  return trimmed
}

export function PolicyDecisionRecord(props: {
  events: PolicyEvent[] | null
  channelNames?: Record<number, string>
}) {
  const { t } = useTranslation()
  return (
    <ol aria-label={t('Request decision flow')} className='space-y-0 text-sm'>
      {(props.events ?? []).map((event, index) => {
        const formattedUpstreamError =
          event.decision.action === 'failure' && event.upstream_error
            ? formatUpstreamErrorText(event.upstream_error)
            : ''
        return (
          <li
            key={`${event.attempt}:${event.channel_id ?? 0}:${event.decision.action}:${event.decision.reason}:${event.rule ?? ''}`}
            className='border-border relative ml-3 min-w-0 border-l pb-5 pl-6 last:border-transparent last:pb-1'
          >
            <span
              aria-hidden='true'
              className='border-border bg-background absolute -left-3 flex size-6 items-center justify-center rounded-full border text-xs'
            >
              {index + 1}
            </span>
            <p
              className={
                event.decision.action === 'stop'
                  ? 'font-medium text-amber-700 dark:text-amber-400'
                  : 'font-medium'
              }
            >
              {policyLabel(t, event.decision.reason)}
              {event.channel_id
                ? ` · ${props.channelNames?.[event.channel_id] ?? `#${event.channel_id}`}`
                : ''}
              {event.status && event.decision.action === 'failure'
                ? ` · HTTP ${event.status}`
                : ''}
            </p>
            <p className='text-muted-foreground text-xs break-words'>
              {event.attempt > 0 ? `${t('Attempt')} ${event.attempt} · ` : ''}
              {event.elapsed_ms} ms
              {event.group ? ` · ${event.group}` : ''}
              {event.rule ? ` · ${event.rule}` : ''}
              {event.status ? ` · HTTP ${event.status}` : ''}
              {event.decision.action === 'failure' && event.error_code
                ? ` · ${event.error_code}`
                : ''}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Source')}: {policyLabel(t, event.decision.source)}
              {event.health ? ` · ${policyLabel(t, event.health)}` : ''}
            </p>
            {formattedUpstreamError ? (
              <details className='bg-background/60 mt-1.5 min-w-0 rounded border px-2.5 py-1.5'>
                <summary className='text-muted-foreground hover:text-foreground cursor-pointer text-xs font-medium select-none'>
                  {t('View upstream error response')}
                </summary>
                <div className='relative mt-1.5 min-w-0'>
                  <CopyButton
                    value={formattedUpstreamError}
                    className='absolute top-0.5 right-0.5 h-6 w-6'
                    iconClassName='size-3'
                  />
                  <pre className='max-h-40 min-w-0 overflow-x-hidden overflow-y-auto pr-7 font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap sm:wrap-break-word'>
                    {formattedUpstreamError}
                  </pre>
                </div>
              </details>
            ) : null}
          </li>
        )
      })}
    </ol>
  )
}
