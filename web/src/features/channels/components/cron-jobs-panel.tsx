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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Clock,
  Edit3,
  ExternalLink,
  FileTerminal,
  Globe,
  Play,
  Plus,
  RefreshCw,
  Terminal,
  Trash2,
  XCircle,
} from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { useIsCompactScreen } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  type CronJob,
  type CronJobType,
  deleteCronJob,
  getCronJobs,
  runCronJobNow,
  saveCronJob,
} from '../api'

const CRON_PRESETS = [
  { label: 'Every 5m', expr: '*/5 * * * *' },
  { label: 'Every 15m', expr: '*/15 * * * *' },
  { label: 'Every 30m', expr: '*/30 * * * *' },
  { label: 'Hourly', expr: '0 * * * *' },
  { label: 'Every 6h', expr: '0 */6 * * *' },
  { label: 'Daily 03:00', expr: '0 3 * * *' },
]

const EMPTY_JOB_DRAFT: Partial<CronJob> = {
  id: '',
  name: '',
  description: '',
  enabled: true,
  cron_expr: '*/30 * * * *',
  job_type: 'shell_script',
  script: '',
  http_method: 'GET',
  http_headers: '',
  http_body: '',
  working_dir: '',
  timeout_sec: 120,
}

interface CronJobsPanelProps {
  onNavigateTab?: (tab: string) => void
}

export function CronJobsPanel(props: CronJobsPanelProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsCompactScreen()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const jobsQuery = useQuery({
    queryKey: ['channel-cron-jobs'],
    queryFn: async () => requireServerSuccess(await getCronJobs()),
  })

  const jobs = jobsQuery.data?.data?.jobs ?? []

  const [editorOpen, setEditorOpen] = useState(false)
  const [draft, setDraft] = useState<Partial<CronJob>>(EMPTY_JOB_DRAFT)
  const [outputModalJob, setOutputModalJob] = useState<CronJob | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<CronJob | null>(null)
  const [runningJobIds, setRunningJobIds] = useState<Set<string>>(
    () => new Set()
  )
  const [togglingJobIds, setTogglingJobIds] = useState<Set<string>>(
    () => new Set()
  )
  const [expandedJobIds, setExpandedJobIds] = useState<Set<string>>(
    () => new Set()
  )

  const toggleJobExpanded = (id: string) => {
    setExpandedJobIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const allExpanded =
    jobs.length > 0 && jobs.every((job) => expandedJobIds.has(job.id))

  const toggleExpandAll = () => {
    if (allExpanded) {
      setExpandedJobIds(new Set())
    } else {
      setExpandedJobIds(new Set(jobs.map((job) => job.id)))
    }
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: Partial<CronJob>) =>
      requireServerSuccess(await saveCronJob(payload)),
    onSuccess: () => {
      toast.success(t('Scheduled task saved'))
      setEditorOpen(false)
      void queryClient.invalidateQueries({ queryKey: ['channel-cron-jobs'] })
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save scheduled task'))
    },
  })

  const toggleEnabledMutation = useMutation({
    mutationFn: async (payload: CronJob) => {
      setTogglingJobIds((prev) => new Set(prev).add(payload.id))
      return requireServerSuccess(await saveCronJob(payload))
    },
    onSuccess: (_res, variables) => {
      const displayName = variables.built_in
        ? t(variables.name)
        : variables.name
      toast.success(
        variables.enabled
          ? `${t('Scheduled task enabled')}: ${displayName}`
          : `${t('Scheduled task disabled')}: ${displayName}`
      )
      void queryClient.invalidateQueries({ queryKey: ['channel-cron-jobs'] })
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save scheduled task'))
    },
    onSettled: (_data, _err, variables) => {
      setTogglingJobIds((prev) => {
        const next = new Set(prev)
        next.delete(variables.id)
        return next
      })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: string) =>
      requireServerSuccess(await deleteCronJob(id)),
    onSuccess: (_res, id) => {
      toast.success(t('Scheduled task deleted'))
      setDeleteTarget(null)
      setExpandedJobIds((prev) => {
        if (!prev.has(id)) return prev
        const next = new Set(prev)
        next.delete(id)
        return next
      })
      void queryClient.invalidateQueries({ queryKey: ['channel-cron-jobs'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to delete scheduled task'))
    },
  })

  const runNowMutation = useMutation({
    mutationFn: async (id: string) => {
      setRunningJobIds((prev) => new Set(prev).add(id))
      return requireServerSuccess(await runCronJobNow(id))
    },
    onSuccess: (res, id) => {
      const job = res.data?.job
      let displayName = ''
      if (job) {
        displayName = job.built_in ? t(job.name) : job.name
      }
      if (job?.last_status === 'failed') {
        toast.error(`${t('Scheduled task failed')}: ${displayName}`)
      } else {
        const durationLabel =
          job && job.last_duration_ms > 0 ? ` (${job.last_duration_ms}ms)` : ''
        toast.success(
          `${t('Scheduled task executed')}: ${displayName}${durationLabel}`
        )
      }
      setExpandedJobIds((prev) => new Set(prev).add(id))
      void queryClient.invalidateQueries({ queryKey: ['channel-cron-jobs'] })
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['channel-model-price-table'],
      })
      void queryClient.invalidateQueries({ queryKey: ['upstream-accounts'] })
      void queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to execute scheduled task'))
    },
    onSettled: (_data, _err, id) => {
      setRunningJobIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    },
  })

  const openCreateModal = () => {
    setDraft({ ...EMPTY_JOB_DRAFT })
    setEditorOpen(true)
  }

  const openEditModal = (job: CronJob) => {
    setDraft({
      ...EMPTY_JOB_DRAFT,
      ...job,
    })
    setEditorOpen(true)
  }

  const formatUnixTime = (ts: number) => {
    if (!ts || ts <= 0) {
      return '-'
    }
    return new Date(ts * 1000).toLocaleString(intlLocale, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  const getJobTypeLabel = (type: CronJobType) => {
    switch (type) {
      case 'upstream_group_sync':
        return t('Upstream Account & Group Channel Sync')
      case 'upstream_sync':
        return t('Channel Upstream Model & Price Sync')
      case 'channel_test':
        return t('Test All Enabled Channels')
      case 'channel_balance':
        return t('Update All Channel Balances')
      case 'http_request':
        return t('HTTP Webhook / Request')
      case 'shell_script':
      default:
        return t('Custom Script / Command')
    }
  }

  const getDisplayName = (job: CronJob) =>
    job.built_in ? t(job.name) : job.name

  const getDisplayDescription = (job: CronJob) => {
    if (!job.description) return ''
    return job.built_in ? t(job.description) : job.description
  }

  const renderStatusBadge = (job: CronJob, isRunning: boolean) => {
    if (isRunning) {
      return (
        <Badge
          variant='secondary'
          className='gap-1 px-1.5 py-0 text-[11px] text-amber-600 dark:text-amber-400'
        >
          <RefreshCw aria-hidden='true' className='size-3 animate-spin' />
          <span>{t('Running')}</span>
        </Badge>
      )
    }
    if (job.last_run_at <= 0) {
      return (
        <span className='text-muted-foreground text-xs'>
          {t('Never executed')}
        </span>
      )
    }
    if (job.last_status === 'success') {
      return (
        <div className='flex items-center gap-1.5'>
          <CheckCircle2
            aria-hidden='true'
            className='size-3.5 shrink-0 text-emerald-500'
          />
          <span className='font-mono text-xs'>
            {formatUnixTime(job.last_run_at)}
          </span>
          {job.last_duration_ms > 0 ? (
            <span className='text-muted-foreground text-[11px]'>
              ({job.last_duration_ms}ms)
            </span>
          ) : null}
        </div>
      )
    }
    if (job.last_status === 'failed') {
      return (
        <div className='flex items-center gap-1.5'>
          <XCircle
            aria-hidden='true'
            className='size-3.5 shrink-0 text-rose-500'
          />
          <span className='font-mono text-xs'>
            {formatUnixTime(job.last_run_at)}
          </span>
          {job.last_duration_ms > 0 ? (
            <span className='text-muted-foreground text-[11px]'>
              ({job.last_duration_ms}ms)
            </span>
          ) : null}
        </div>
      )
    }
    return (
      <div className='flex items-center gap-1.5'>
        <RefreshCw
          aria-hidden='true'
          className='size-3.5 shrink-0 animate-spin text-amber-500'
        />
        <span className='font-mono text-xs'>
          {formatUnixTime(job.last_run_at)}
        </span>
      </div>
    )
  }

  const getRelatedChannelLink = (
    type: CronJobType
  ): { tab: string; label: string } | null => {
    switch (type) {
      case 'upstream_group_sync':
        return {
          tab: 'upstream-accounts',
          label: t('Configure Upstream Accounts'),
        }
      case 'upstream_sync':
        return { tab: 'upstream-sync', label: t('Configure Sync Settings') }
      case 'channel_test':
      case 'channel_balance':
        return { tab: 'all', label: t('View Channels') }
      default:
        return null
    }
  }

  const enabledJobCount = jobs.filter((job) => job.enabled).length

  const renderJobDetailsContent = (job: CronJob) => {
    const description = getDisplayDescription(job)
    const relatedLink = props.onNavigateTab
      ? getRelatedChannelLink(job.job_type)
      : null
    return (
      <div className='flex flex-col gap-3 text-xs'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          {description ? (
            <p className='text-muted-foreground'>{description}</p>
          ) : (
            <span />
          )}
          {relatedLink && props.onNavigateTab ? (
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => props.onNavigateTab?.(relatedLink.tab)}
              className='h-7 gap-1.5 px-2.5 text-xs'
            >
              <ExternalLink aria-hidden='true' className='size-3' />
              <span>{relatedLink.label}</span>
            </Button>
          ) : null}
        </div>

        <div className='grid grid-cols-1 gap-2.5 sm:grid-cols-3'>
          <div className='bg-muted/30 flex flex-col gap-0.5 rounded-md px-2.5 py-2'>
            <span className='text-muted-foreground text-[11px]'>
              {t('Task Type')}
            </span>
            <span className='font-medium'>{getJobTypeLabel(job.job_type)}</span>
          </div>
          <div className='bg-muted/30 flex flex-col gap-0.5 rounded-md px-2.5 py-2'>
            <span className='text-muted-foreground text-[11px]'>
              {t('Timeout (sec)')}
            </span>
            <span className='font-mono'>{job.timeout_sec || 120}s</span>
          </div>
          {job.job_type === 'http_request' ? (
            <div className='bg-muted/30 flex flex-col gap-0.5 rounded-md px-2.5 py-2'>
              <span className='text-muted-foreground text-[11px]'>
                {t('HTTP Method')}
              </span>
              <span className='font-mono'>{job.http_method || 'GET'}</span>
            </div>
          ) : (
            <div className='bg-muted/30 flex flex-col gap-0.5 rounded-md px-2.5 py-2'>
              <span className='text-muted-foreground text-[11px]'>
                {t('Working Directory')}
              </span>
              <span className='truncate font-mono'>
                {job.working_dir || '-'}
              </span>
            </div>
          )}
        </div>

        {job.script ? (
          <div className='flex flex-col gap-1'>
            <span className='text-muted-foreground text-[11px] font-medium'>
              {job.job_type === 'http_request'
                ? t('Target URL')
                : t('Script / Command')}
            </span>
            <pre className='bg-muted/40 max-h-32 overflow-auto rounded-md p-2.5 font-mono text-[11px] break-all whitespace-pre-wrap'>
              {job.script}
            </pre>
          </div>
        ) : null}

        {job.job_type === 'http_request' && job.http_headers ? (
          <div className='flex flex-col gap-1'>
            <span className='text-muted-foreground text-[11px] font-medium'>
              {t('HTTP Headers')}
            </span>
            <pre className='bg-muted/40 max-h-24 overflow-auto rounded-md p-2 font-mono text-[11px] break-all whitespace-pre-wrap'>
              {job.http_headers}
            </pre>
          </div>
        ) : null}

        {job.job_type === 'http_request' && job.http_body ? (
          <div className='flex flex-col gap-1'>
            <span className='text-muted-foreground text-[11px] font-medium'>
              {t('HTTP Body')}
            </span>
            <pre className='bg-muted/40 max-h-24 overflow-auto rounded-md p-2 font-mono text-[11px] break-all whitespace-pre-wrap'>
              {job.http_body}
            </pre>
          </div>
        ) : null}

        <div className='flex flex-col gap-1'>
          <div className='flex items-center justify-between gap-2'>
            <span className='text-muted-foreground text-[11px] font-medium'>
              {t('Last Execution Output')}
            </span>
            {job.last_output ? (
              <Button
                variant='ghost'
                size='sm'
                onClick={() => setOutputModalJob(job)}
                className='h-6 gap-1 px-2 text-[11px]'
              >
                <FileTerminal aria-hidden='true' className='size-3' />
                <span>{t('View Full Logs')}</span>
              </Button>
            ) : null}
          </div>
          {job.last_output ? (
            <pre className='bg-muted/50 max-h-36 overflow-auto rounded-md p-2.5 font-mono text-[11px] break-all whitespace-pre-wrap'>
              {job.last_output}
            </pre>
          ) : (
            <span className='text-muted-foreground text-[11px]'>
              {t('No execution output yet')}
            </span>
          )}
        </div>
      </div>
    )
  }

  if (jobsQuery.isLoading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-20 w-full rounded-xl' />
        <Skeleton className='h-72 w-full rounded-xl' />
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      {/* Header Card */}
      <div className='border-border/60 bg-card flex flex-wrap items-center justify-between gap-3 rounded-xl border p-5'>
        <div className='flex items-center gap-2.5'>
          <Clock aria-hidden='true' className='text-primary size-5' />
          <div>
            <div className='flex flex-wrap items-center gap-2'>
              <h3 className='text-sm font-semibold'>{t('Scheduled Tasks')}</h3>
              {jobs.length > 0 ? (
                <Badge
                  variant={enabledJobCount > 0 ? 'secondary' : 'outline'}
                  className='px-1.5 py-0 text-[11px]'
                >
                  {t('{{enabled}}/{{total}} active', {
                    enabled: enabledJobCount,
                    total: jobs.length,
                  })}
                </Badge>
              ) : null}
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Schedule automated maintenance tasks, upstream synchronization, HTTP webhooks, or custom shell scripts with standard 5-field cron expressions'
              )}
            </p>
          </div>
        </div>

        <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
          {jobs.length > 0 ? (
            <Button
              variant='outline'
              size='sm'
              onClick={toggleExpandAll}
              className='flex-1 gap-1.5 sm:flex-initial'
            >
              {allExpanded ? (
                <ChevronsDownUp aria-hidden='true' className='size-3.5' />
              ) : (
                <ChevronsUpDown aria-hidden='true' className='size-3.5' />
              )}
              <span>{allExpanded ? t('Collapse All') : t('Expand All')}</span>
            </Button>
          ) : null}
          <Button
            variant='outline'
            size='sm'
            disabled={jobsQuery.isFetching}
            onClick={() =>
              void queryClient.invalidateQueries({
                queryKey: ['channel-cron-jobs'],
              })
            }
            className='flex-1 gap-1.5 sm:flex-initial'
          >
            <RefreshCw
              aria-hidden='true'
              className={`size-3.5 ${jobsQuery.isFetching ? 'animate-spin' : ''}`}
            />
            <span>{t('Refresh')}</span>
          </Button>
          <Button
            size='sm'
            onClick={openCreateModal}
            className='flex-1 gap-1.5 sm:flex-initial'
          >
            <Plus aria-hidden='true' className='size-3.5' />
            <span>{t('New Scheduled Task')}</span>
          </Button>
        </div>
      </div>

      {/* Jobs Table / Mobile Cards */}
      <div className='border-border/60 bg-card flex flex-col gap-3 rounded-xl border p-4'>
        {jobs.length === 0 ? (
          <EmptyState
            icon={Clock}
            title={t('No scheduled tasks')}
            description={t(
              'Create a scheduled task to automate upstream syncs, webhooks, or custom scripts.'
            )}
            action={
              <Button size='sm' onClick={openCreateModal} className='gap-1.5'>
                <Plus aria-hidden='true' className='size-3.5' />
                <span>{t('New Scheduled Task')}</span>
              </Button>
            }
          />
        ) : null}
        {jobs.length > 0 && isMobile ? (
          <div className='flex flex-col gap-2.5'>
            {jobs.map((job) => {
              const isRunning =
                runningJobIds.has(job.id) || job.last_status === 'running'
              const isToggling = togglingJobIds.has(job.id)
              const isExpanded = expandedJobIds.has(job.id)
              const displayName = getDisplayName(job)

              return (
                <Collapsible
                  key={job.id}
                  open={isExpanded}
                  onOpenChange={() => toggleJobExpanded(job.id)}
                  className='border-border/60 bg-background/60 rounded-lg border p-3'
                >
                  <div className='flex items-start justify-between gap-2'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <CollapsibleTrigger
                        render={
                          <Button
                            variant='ghost'
                            size='sm'
                            aria-label={`${isExpanded ? t('Collapse details') : t('Expand details')} ${displayName}`}
                            className='size-6 shrink-0 p-0'
                          />
                        }
                      >
                        {isExpanded ? (
                          <ChevronDown aria-hidden='true' className='size-4' />
                        ) : (
                          <ChevronRight aria-hidden='true' className='size-4' />
                        )}
                      </CollapsibleTrigger>
                      <Switch
                        checked={job.enabled}
                        disabled={isToggling}
                        aria-label={`${t('Toggle')} ${displayName}`}
                        onCheckedChange={(val) =>
                          toggleEnabledMutation.mutate({
                            ...job,
                            enabled: Boolean(val),
                          })
                        }
                        className='shrink-0'
                      />
                      <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
                        <span className='truncate text-xs font-semibold'>
                          {displayName}
                        </span>
                        {job.built_in ? (
                          <Badge
                            variant='secondary'
                            className='px-1.5 py-0 text-[10px]'
                          >
                            {t('Built-in')}
                          </Badge>
                        ) : null}
                      </div>
                    </div>
                    <Badge
                      variant='outline'
                      className='shrink-0 font-mono text-[11px]'
                    >
                      {job.cron_expr}
                    </Badge>
                  </div>

                  <div className='mt-2 flex flex-wrap items-center justify-between gap-2 pl-8'>
                    <div className='flex items-center gap-2'>
                      {renderStatusBadge(job, isRunning)}
                    </div>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={isRunning}
                      onClick={() => runNowMutation.mutate(job.id)}
                      className='h-7 gap-1 px-2.5 text-xs'
                    >
                      {isRunning ? (
                        <RefreshCw
                          aria-hidden='true'
                          className='size-3 animate-spin'
                        />
                      ) : (
                        <Play aria-hidden='true' className='size-3' />
                      )}
                      <span>{isRunning ? t('Running') : t('Run Now')}</span>
                    </Button>
                  </div>

                  <CollapsibleContent className='border-border/40 mt-3 flex flex-col gap-3 border-t pt-3'>
                    <div className='flex items-center justify-between text-xs'>
                      <span className='text-muted-foreground'>
                        {t('Next Run')}
                      </span>
                      <span className='font-mono'>
                        {job.enabled ? formatUnixTime(job.next_run_at) : '-'}
                      </span>
                    </div>

                    {renderJobDetailsContent(job)}

                    <div className='border-border/40 flex flex-wrap items-center justify-end gap-1.5 border-t pt-2'>
                      {job.last_output ? (
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => setOutputModalJob(job)}
                          className='h-7 gap-1 px-2 text-xs'
                        >
                          <FileTerminal
                            aria-hidden='true'
                            className='size-3.5'
                          />
                          <span>{t('Logs')}</span>
                        </Button>
                      ) : null}
                      <Button
                        variant='ghost'
                        size='sm'
                        aria-label={`${t('Edit CRON Task')} ${displayName}`}
                        onClick={() => openEditModal(job)}
                        className='h-7 gap-1 px-2 text-xs'
                      >
                        <Edit3 aria-hidden='true' className='size-3.5' />
                        <span>{t('Edit')}</span>
                      </Button>
                      {!job.built_in ? (
                        <Button
                          variant='ghost'
                          size='sm'
                          aria-label={`${t('Delete CRON Task')} ${job.name}`}
                          onClick={() => setDeleteTarget(job)}
                          className='h-7 gap-1 px-2 text-xs text-rose-500 hover:text-rose-600'
                        >
                          <Trash2 aria-hidden='true' className='size-3.5' />
                          <span>{t('Delete')}</span>
                        </Button>
                      ) : null}
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              )
            })}
          </div>
        ) : null}
        {jobs.length > 0 && !isMobile ? (
          <div className='border-border/50 overflow-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-10' />
                  <TableHead className='w-20 text-center'>
                    {t('Enabled')}
                  </TableHead>
                  <TableHead>{t('Task Name')}</TableHead>
                  <TableHead>{t('Task Type')}</TableHead>
                  <TableHead>{t('Cron Schedule')}</TableHead>
                  <TableHead>{t('Next Run')}</TableHead>
                  <TableHead>{t('Last Run & Status')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {jobs.map((job) => {
                  const isRunning =
                    runningJobIds.has(job.id) || job.last_status === 'running'
                  const isToggling = togglingJobIds.has(job.id)
                  const isExpanded = expandedJobIds.has(job.id)
                  const displayName = getDisplayName(job)

                  return (
                    <Fragment key={job.id}>
                      <TableRow>
                        <TableCell className='pr-0'>
                          <Button
                            variant='ghost'
                            size='sm'
                            aria-expanded={isExpanded}
                            aria-label={`${isExpanded ? t('Collapse details') : t('Expand details')} ${displayName}`}
                            onClick={() => toggleJobExpanded(job.id)}
                            className='size-7 p-0'
                          >
                            {isExpanded ? (
                              <ChevronDown
                                aria-hidden='true'
                                className='size-4'
                              />
                            ) : (
                              <ChevronRight
                                aria-hidden='true'
                                className='size-4'
                              />
                            )}
                          </Button>
                        </TableCell>
                        <TableCell className='text-center'>
                          <Switch
                            checked={job.enabled}
                            disabled={isToggling}
                            aria-label={`${t('Toggle')} ${displayName}`}
                            onCheckedChange={(val) =>
                              toggleEnabledMutation.mutate({
                                ...job,
                                enabled: Boolean(val),
                              })
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <div className='flex items-center gap-1.5'>
                            <button
                              type='button'
                              onClick={() => toggleJobExpanded(job.id)}
                              className='hover:text-primary cursor-pointer text-left text-xs font-medium transition-colors'
                            >
                              {displayName}
                            </button>
                            {job.built_in ? (
                              <Badge
                                variant='secondary'
                                className='px-1.5 py-0 text-[10px]'
                              >
                                {t('Built-in')}
                              </Badge>
                            ) : null}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className='flex items-center gap-1.5 text-xs'>
                            {job.job_type === 'shell_script' ? (
                              <Terminal
                                aria-hidden='true'
                                className='text-primary size-3.5'
                              />
                            ) : null}
                            {job.job_type === 'http_request' ? (
                              <Globe
                                aria-hidden='true'
                                className='text-primary size-3.5'
                              />
                            ) : null}
                            {job.job_type !== 'shell_script' &&
                            job.job_type !== 'http_request' ? (
                              <RefreshCw
                                aria-hidden='true'
                                className='text-primary size-3.5'
                              />
                            ) : null}
                            <span>{getJobTypeLabel(job.job_type)}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant='outline'
                            className='font-mono text-xs'
                          >
                            {job.cron_expr}
                          </Badge>
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {job.enabled ? formatUnixTime(job.next_run_at) : '-'}
                        </TableCell>
                        <TableCell className='text-xs'>
                          {renderStatusBadge(job, isRunning)}
                        </TableCell>
                        <TableCell className='text-right'>
                          <div className='flex items-center justify-end gap-1.5'>
                            <Button
                              variant='outline'
                              size='sm'
                              disabled={isRunning}
                              onClick={() => runNowMutation.mutate(job.id)}
                              className='h-7 gap-1 px-2 text-xs'
                            >
                              {isRunning ? (
                                <RefreshCw
                                  aria-hidden='true'
                                  className='size-3 animate-spin'
                                />
                              ) : (
                                <Play aria-hidden='true' className='size-3' />
                              )}
                              <span>
                                {isRunning ? t('Running') : t('Run Now')}
                              </span>
                            </Button>
                            {job.last_output ? (
                              <Button
                                variant='ghost'
                                size='sm'
                                onClick={() => setOutputModalJob(job)}
                                className='h-7 gap-1 px-2 text-xs'
                              >
                                <FileTerminal
                                  aria-hidden='true'
                                  className='size-3.5'
                                />
                                <span>{t('Logs')}</span>
                              </Button>
                            ) : null}
                            <Button
                              variant='ghost'
                              size='sm'
                              aria-label={`${t('Edit CRON Task')} ${displayName}`}
                              onClick={() => openEditModal(job)}
                              className='h-7 px-2 text-xs'
                            >
                              <Edit3 aria-hidden='true' className='size-3.5' />
                            </Button>
                            {!job.built_in ? (
                              <Button
                                variant='ghost'
                                size='sm'
                                aria-label={`${t('Delete CRON Task')} ${job.name}`}
                                onClick={() => setDeleteTarget(job)}
                                className='h-7 px-2 text-xs text-rose-500 hover:text-rose-600'
                              >
                                <Trash2
                                  aria-hidden='true'
                                  className='size-3.5'
                                />
                              </Button>
                            ) : null}
                          </div>
                        </TableCell>
                      </TableRow>
                      {isExpanded ? (
                        <TableRow className='bg-muted/20 hover:bg-muted/20'>
                          <TableCell colSpan={8} className='px-6 py-4'>
                            {renderJobDetailsContent(job)}
                          </TableCell>
                        </TableRow>
                      ) : null}
                    </Fragment>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        ) : null}
      </div>

      {/* Create / Edit CRON Job Modal */}
      <Dialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        title={draft.id ? t('Edit CRON Task') : t('New CRON Task')}
        description={t(
          'Configure a 5-field cron schedule (min hour dom month dow) and attach a built-in task or custom script'
        )}
        footer={
          <div className='flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row sm:items-center sm:justify-end'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setEditorOpen(false)}
              className='w-full sm:w-auto'
            >
              {t('Cancel')}
            </Button>
            <Button
              size='sm'
              disabled={saveMutation.isPending}
              onClick={() => saveMutation.mutate(draft)}
              className='w-full sm:w-auto'
            >
              {t('Save CRON Task')}
            </Button>
          </div>
        }
      >
        <div className='flex max-h-[70dvh] flex-col gap-4 overflow-y-auto px-0.5 py-1'>
          <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
            <div className='flex flex-col gap-1.5'>
              <Label className='text-xs font-semibold'>{t('Task Name')}</Label>
              <Input
                value={draft.name ?? ''}
                onChange={(e) =>
                  setDraft((p) => ({ ...p, name: e.target.value }))
                }
                placeholder={t('e.g. Custom Backup or Health Script')}
                className='h-8 text-xs'
              />
            </div>
            <div className='flex flex-col gap-1.5'>
              <Label className='text-xs font-semibold'>{t('Task Type')}</Label>
              <NativeSelect
                size='sm'
                disabled={Boolean(draft.built_in)}
                value={draft.job_type ?? 'shell_script'}
                onChange={(e) =>
                  setDraft((p) => ({
                    ...p,
                    job_type: e.target.value as CronJobType,
                  }))
                }
              >
                <NativeSelectOption value='shell_script'>
                  {t('Custom Script / Command')}
                </NativeSelectOption>
                <NativeSelectOption value='http_request'>
                  {t('HTTP Webhook / Request')}
                </NativeSelectOption>
                <NativeSelectOption value='upstream_group_sync'>
                  {t('Upstream Account & Group Channel Sync')}
                </NativeSelectOption>
                <NativeSelectOption value='upstream_sync'>
                  {t('Channel Upstream Model & Price Sync')}
                </NativeSelectOption>
                <NativeSelectOption value='channel_test'>
                  {t('Test All Enabled Channels')}
                </NativeSelectOption>
                <NativeSelectOption value='channel_balance'>
                  {t('Update All Channel Balances')}
                </NativeSelectOption>
              </NativeSelect>
            </div>
          </div>

          <div className='flex flex-col gap-1.5'>
            <Label className='text-xs font-semibold'>{t('Description')}</Label>
            <Input
              value={draft.description ?? ''}
              onChange={(e) =>
                setDraft((p) => ({ ...p, description: e.target.value }))
              }
              placeholder={t('Optional task description')}
              className='h-8 text-xs'
            />
          </div>

          <div className='flex flex-col gap-2'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <Label className='text-xs font-semibold'>
                {t('Cron Expression (5-Field or @every)')}
              </Label>
              <div className='flex flex-wrap gap-1'>
                {CRON_PRESETS.map((preset) => (
                  <Button
                    key={preset.expr}
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() =>
                      setDraft((p) => ({ ...p, cron_expr: preset.expr }))
                    }
                    className='h-6 px-2 text-[11px]'
                  >
                    {t(preset.label)}
                  </Button>
                ))}
              </div>
            </div>
            <Input
              value={draft.cron_expr ?? ''}
              onChange={(e) =>
                setDraft((p) => ({ ...p, cron_expr: e.target.value }))
              }
              placeholder='*/30 * * * *'
              className='h-8 font-mono text-xs'
            />
          </div>

          {draft.job_type === 'shell_script' ? (
            <>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs font-semibold'>
                  {t('Shell / PowerShell Script or Command')}
                </Label>
                <Textarea
                  rows={5}
                  value={draft.script ?? ''}
                  onChange={(e) =>
                    setDraft((p) => ({ ...p, script: e.target.value }))
                  }
                  placeholder={t(
                    'Enter shell/PowerShell commands or script path to execute on schedule...'
                  )}
                  className='font-mono text-xs'
                />
              </div>
              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <div className='flex flex-col gap-1.5'>
                  <Label className='text-xs font-semibold'>
                    {t('Working Directory (Optional)')}
                  </Label>
                  <Input
                    value={draft.working_dir ?? ''}
                    onChange={(e) =>
                      setDraft((p) => ({ ...p, working_dir: e.target.value }))
                    }
                    placeholder={t('Default workspace directory')}
                    className='h-8 font-mono text-xs'
                  />
                </div>
                <div className='flex flex-col gap-1.5'>
                  <Label className='text-xs font-semibold'>
                    {t('Timeout (Seconds)')}
                  </Label>
                  <Input
                    type='number'
                    min={5}
                    value={draft.timeout_sec ?? 120}
                    onChange={(e) =>
                      setDraft((p) => ({
                        ...p,
                        timeout_sec: Math.max(5, Number(e.target.value) || 120),
                      }))
                    }
                    className='h-8 text-xs'
                  />
                </div>
              </div>
            </>
          ) : null}

          {draft.job_type === 'http_request' ? (
            <>
              <div className='grid grid-cols-1 gap-3 sm:grid-cols-4'>
                <div className='flex flex-col gap-1.5'>
                  <Label className='text-xs font-semibold'>
                    {t('HTTP Method')}
                  </Label>
                  <NativeSelect
                    size='sm'
                    value={draft.http_method ?? 'GET'}
                    onChange={(e) =>
                      setDraft((p) => ({ ...p, http_method: e.target.value }))
                    }
                  >
                    <NativeSelectOption value='GET'>GET</NativeSelectOption>
                    <NativeSelectOption value='POST'>POST</NativeSelectOption>
                    <NativeSelectOption value='PUT'>PUT</NativeSelectOption>
                  </NativeSelect>
                </div>
                <div className='flex flex-col gap-1.5 sm:col-span-3'>
                  <Label className='text-xs font-semibold'>
                    {t('Target URL')}
                  </Label>
                  <Input
                    value={draft.script ?? ''}
                    onChange={(e) =>
                      setDraft((p) => ({ ...p, script: e.target.value }))
                    }
                    placeholder='https://example.com/webhook'
                    className='h-8 font-mono text-xs'
                  />
                </div>
              </div>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs font-semibold'>
                  {t('Request Headers (JSON Optional)')}
                </Label>
                <Input
                  value={draft.http_headers ?? ''}
                  onChange={(e) =>
                    setDraft((p) => ({ ...p, http_headers: e.target.value }))
                  }
                  placeholder='{"Authorization": "Bearer ..."}'
                  className='h-8 font-mono text-xs'
                />
              </div>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs font-semibold'>
                  {t('Request Body (Optional)')}
                </Label>
                <Textarea
                  rows={3}
                  value={draft.http_body ?? ''}
                  onChange={(e) =>
                    setDraft((p) => ({ ...p, http_body: e.target.value }))
                  }
                  className='font-mono text-xs'
                />
              </div>
            </>
          ) : null}

          <div className='border-border/50 bg-muted/20 flex items-center justify-between rounded-lg border p-3'>
            <Label className='text-xs font-semibold'>
              {t('Enable Scheduled Execution')}
            </Label>
            <Switch
              checked={Boolean(draft.enabled)}
              onCheckedChange={(val) =>
                setDraft((p) => ({ ...p, enabled: Boolean(val) }))
              }
            />
          </div>
        </div>
      </Dialog>

      {/* Execution Output Modal */}
      <Dialog
        open={Boolean(outputModalJob)}
        onOpenChange={(open) => {
          if (!open) {
            setOutputModalJob(null)
          }
        }}
        title={`${t('Execution Output')}: ${outputModalJob?.name ?? ''}`}
        description={
          outputModalJob
            ? `${formatUnixTime(outputModalJob.last_run_at)} (${outputModalJob.last_duration_ms}ms)`
            : ''
        }
      >
        <pre className='bg-muted/40 border-border/50 max-h-96 overflow-auto rounded-lg border p-3 font-mono text-xs whitespace-pre-wrap'>
          {outputModalJob?.last_output || t('No output recorded')}
        </pre>
      </Dialog>

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null)
          }
        }}
        title={t('Delete CRON Task')}
        desc={t('Are you sure you want to delete this scheduled CRON task?')}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteTarget) {
            deleteMutation.mutate(deleteTarget.id)
          }
        }}
      />
    </div>
  )
}
