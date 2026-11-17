/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StatusBadge } from '@/components/status-badge'

import type {
  AdminUpstreamErrorEntry,
  UserFinalErrorEntry,
} from '../../lib/error-response'
import { DetailSection } from './log-detail-layout'

function formatChannelBadgeLabel(
  channelId: number | undefined,
  logChannel: number,
  logChannelName?: string | null
): string {
  if (channelId == null || channelId <= 0) {
    return ''
  }
  if (channelId === logChannel && logChannelName) {
    return `#${channelId} (${logChannelName})`
  }
  return `#${channelId}`
}

export function AdminUpstreamErrorSection(props: {
  entries: AdminUpstreamErrorEntry[]
  logChannel: number
  logChannelName?: string | null
}) {
  const { t } = useTranslation()

  return (
    <DetailSection
      icon={<AlertTriangle className='size-3.5' aria-hidden='true' />}
      label={t('Upstream Error Response')}
      variant='danger'
    >
      <div className='min-w-0 space-y-2'>
        {props.entries.map((entry) => {
          const channelLabel = formatChannelBadgeLabel(
            entry.channelId,
            props.logChannel,
            props.logChannelName
          )
          const hasBadges =
            entry.attempt != null ||
            channelLabel !== '' ||
            (entry.status != null && entry.status > 0) ||
            Boolean(entry.errorCode)

          return (
            <div key={entry.key} className='min-w-0 space-y-1.5'>
              {hasBadges ? (
                <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
                  {entry.attempt != null ? (
                    <StatusBadge
                      label={`${t('Attempt')} ${entry.attempt}`}
                      variant='neutral'
                      size='sm'
                      copyable={false}
                    />
                  ) : null}
                  {channelLabel !== '' ? (
                    <StatusBadge
                      label={channelLabel}
                      variant='neutral'
                      size='sm'
                      copyable={false}
                    />
                  ) : null}
                  {entry.status != null && entry.status > 0 ? (
                    <StatusBadge
                      label={`HTTP ${entry.status}`}
                      variant='red'
                      size='sm'
                      copyable={false}
                    />
                  ) : null}
                  {entry.errorCode ? (
                    <StatusBadge
                      label={entry.errorCode}
                      variant='orange'
                      size='sm'
                      copyable={false}
                    />
                  ) : null}
                </div>
              ) : null}
              <div className='bg-background/70 relative min-w-0 overflow-hidden rounded border p-2.5'>
                <CopyButton
                  value={entry.response}
                  className='absolute top-1.5 right-1.5 h-6 w-6'
                  iconClassName='size-3'
                />
                <pre className='max-h-48 min-w-0 overflow-x-hidden overflow-y-auto pr-7 font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap sm:wrap-break-word'>
                  {entry.response}
                </pre>
              </div>
            </div>
          )
        })}
      </div>
    </DetailSection>
  )
}

export function UserFinalErrorSection(props: { entry: UserFinalErrorEntry }) {
  const { t } = useTranslation()
  const hasBadges =
    (props.entry.status != null && props.entry.status > 0) ||
    Boolean(props.entry.errorCode) ||
    Boolean(props.entry.errorType)

  return (
    <DetailSection
      icon={<AlertTriangle className='size-3.5' aria-hidden='true' />}
      label={t('Final Error Response')}
      variant='danger'
    >
      <div className='min-w-0 space-y-1.5'>
        {hasBadges ? (
          <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
            {props.entry.status != null && props.entry.status > 0 ? (
              <StatusBadge
                label={`HTTP ${props.entry.status}`}
                variant='red'
                size='sm'
                copyable={false}
              />
            ) : null}
            {props.entry.errorCode ? (
              <StatusBadge
                label={props.entry.errorCode}
                variant='orange'
                size='sm'
                copyable={false}
              />
            ) : null}
            {props.entry.errorType &&
            props.entry.errorType !== props.entry.errorCode ? (
              <StatusBadge
                label={props.entry.errorType}
                variant='neutral'
                size='sm'
                copyable={false}
              />
            ) : null}
          </div>
        ) : null}
        <div className='bg-background/70 relative min-w-0 overflow-hidden rounded border p-2.5'>
          <CopyButton
            value={props.entry.response}
            className='absolute top-1.5 right-1.5 h-6 w-6'
            iconClassName='size-3'
          />
          <pre className='max-h-48 min-w-0 overflow-x-hidden overflow-y-auto pr-7 font-mono text-[11px] leading-relaxed break-all whitespace-pre-wrap sm:wrap-break-word'>
            {props.entry.response}
          </pre>
        </div>
      </div>
    </DetailSection>
  )
}
