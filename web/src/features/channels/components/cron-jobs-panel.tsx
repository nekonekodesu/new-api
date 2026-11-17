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
  Clock,
  Edit3,
  FileTerminal,
  Globe,
  Play,
  Plus,
  RefreshCw,
  Terminal,
  Trash2,
  XCircle,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { useMediaQuery } from '@/hooks'
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

export function CronJobsPanel() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 640px)')
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
  const [runningJobId, setRunningJobId] = useState<string | null>(null)

  const saveMutation = useMutation({
    mutationFn: async (payload: Partial<CronJob>) =>
      requireServerSuccess(await saveCronJob(payload)),
    onSuccess: () => {
      toast.success(t('CRON task saved'))
      setEditorOpen(false)
      void queryClient.invalidateQueries({ queryKey: ['channel-cron-jobs'] })
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save CRON task'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: string) =>
      requireServerSuccess(await deleteCronJob(id)),
    onSuccess: () => {
      toast.success(t('CRON task deleted'))
      setDeleteTarget(null)
      void queryClient.invalidateQueries({ queryKey: ['channel-cron-jobs'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to delete CRON task'))
    },
  })

  const runNowMutation = useMutation({
    mutationFn: async (id: string) => {
      setRunningJobId(id)
      return requireServerSuccess(await runCronJobNow(id))
    },
    onSuccess: (res) => {
      setRunningJobId(null)
      const job = res.data?.job
      if (job?.last_status === 'failed') {
        toast.error(`${t('CRON task failed')}: ${job.name}`)
      } else {
        toast.success(`${t('CRON task executed')}: ${job?.name ?? ''}`)
      }
      if (job) {
        setOutputModalJob(job)
      }
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
      setRunningJobId(null)
      handleServerError(err, t('Failed to execute CRON task'))
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
            <h3 className='text-sm font-semibold'>
              {t('CRON Scheduled Tasks')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Schedule automated maintenance tasks, upstream synchronization, HTTP webhooks, or custom shell scripts with standard 5-field cron expressions'
              )}
            </p>
          </div>
        </div>

        <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
          <Button
            variant='outline'
            size='sm'
            onClick={() =>
              void queryClient.invalidateQueries({
                queryKey: ['channel-cron-jobs'],
              })
            }
            className='flex-1 gap-1.5 sm:flex-initial'
          >
            <RefreshCw aria-hidden='true' className='size-3.5' />
            <span>{t('Refresh')}</span>
          </Button>
          <Button
            size='sm'
            onClick={openCreateModal}
            className='flex-1 gap-1.5 sm:flex-initial'
          >
            <Plus aria-hidden='true' className='size-3.5' />
            <span>{t('New CRON Task')}</span>
          </Button>
        </div>
      </div>

      {/* Jobs Table / Mobile Cards */}
      <div className='border-border/60 bg-card flex flex-col gap-3 rounded-xl border p-4'>
        {isMobile ? (
          <div className='flex flex-col gap-2.5'>
            {jobs.map((job) => {
              const isRunning =
                runningJobId === job.id || job.last_status === 'running'
              return (
                <div
                  key={job.id}
                  className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
                >
                  <div className='flex items-start justify-between gap-2'>
                    <div className='flex min-w-0 items-start gap-2.5'>
                      <Switch
                        checked={job.enabled}
                        onCheckedChange={(val) =>
                          saveMutation.mutate({
                            ...job,
                            enabled: Boolean(val),
                          })
                        }
                        className='mt-0.5 shrink-0'
                      />
                      <div className='flex min-w-0 flex-col gap-0.5'>
                        <div className='flex flex-wrap items-center gap-1.5'>
                          <span className='truncate text-xs font-semibold'>
                            {job.built_in ? t(job.name) : job.name}
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
                        {job.description ? (
                          <span className='text-muted-foreground text-[11px]'>
                            {job.built_in
                              ? t(job.description)
                              : job.description}
                          </span>
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

                  {job.script ? (
                    <div className='bg-muted/30 text-muted-foreground truncate rounded px-2 py-1 font-mono text-[11px]'>
                      {job.script}
                    </div>
                  ) : null}

                  <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 text-xs'>
                    <div className='col-span-2 flex items-center gap-1.5'>
                      {job.job_type === 'shell_script' ? (
                        <Terminal
                          aria-hidden='true'
                          className='text-primary size-3.5 shrink-0'
                        />
                      ) : null}
                      {job.job_type === 'http_request' ? (
                        <Globe
                          aria-hidden='true'
                          className='text-primary size-3.5 shrink-0'
                        />
                      ) : null}
                      {job.job_type !== 'shell_script' &&
                      job.job_type !== 'http_request' ? (
                        <RefreshCw
                          aria-hidden='true'
                          className='text-primary size-3.5 shrink-0'
                        />
                      ) : null}
                      <span className='truncate font-medium'>
                        {getJobTypeLabel(job.job_type)}
                      </span>
                    </div>
                    <div className='flex flex-col gap-0.5'>
                      <span className='text-muted-foreground text-[10px]'>
                        {t('Next Run')}
                      </span>
                      <span className='font-mono text-[11px]'>
                        {job.enabled ? formatUnixTime(job.next_run_at) : '-'}
                      </span>
                    </div>
                    <div className='flex flex-col gap-0.5'>
                      <span className='text-muted-foreground text-[10px]'>
                        {t('Last Run & Status')}
                      </span>
                      {job.last_run_at > 0 ? (
                        <div className='flex items-center gap-1'>
                          {job.last_status === 'success' ? (
                            <CheckCircle2
                              aria-hidden='true'
                              className='size-3 shrink-0 text-emerald-500'
                            />
                          ) : null}
                          {job.last_status === 'failed' ? (
                            <XCircle
                              aria-hidden='true'
                              className='size-3 shrink-0 text-rose-500'
                            />
                          ) : null}
                          {job.last_status !== 'success' &&
                          job.last_status !== 'failed' ? (
                            <RefreshCw
                              aria-hidden='true'
                              className='size-3 shrink-0 animate-spin text-amber-500'
                            />
                          ) : null}
                          <span className='truncate font-mono text-[11px]'>
                            {formatUnixTime(job.last_run_at)}
                          </span>
                        </div>
                      ) : (
                        <span className='text-muted-foreground text-[11px]'>
                          {t('Never executed')}
                        </span>
                      )}
                    </div>
                  </div>

                  {job.last_output ? (
                    <span className='text-muted-foreground truncate font-mono text-[11px]'>
                      {job.last_output}
                    </span>
                  ) : null}

                  <div className='border-border/40 flex flex-wrap items-center justify-end gap-1.5 border-t pt-2'>
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
                      <span>{t('Run Now')}</span>
                    </Button>
                    {job.last_output ? (
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => setOutputModalJob(job)}
                        className='h-7 gap-1 px-2 text-xs'
                      >
                        <FileTerminal aria-hidden='true' className='size-3.5' />
                        <span>{t('Logs')}</span>
                      </Button>
                    ) : null}
                    <Button
                      variant='ghost'
                      size='sm'
                      aria-label={`${t('Edit CRON Task')} ${job.built_in ? t(job.name) : job.name}`}
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
                        <Trash2 aria-hidden='true' className='size-3.5' />
                      </Button>
                    ) : null}
                  </div>
                </div>
              )
            })}
          </div>
        ) : (
          <div className='border-border/50 overflow-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
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
                    runningJobId === job.id || job.last_status === 'running'
                  return (
                    <TableRow key={job.id}>
                      <TableCell className='text-center'>
                        <Switch
                          checked={job.enabled}
                          onCheckedChange={(val) =>
                            saveMutation.mutate({
                              ...job,
                              enabled: Boolean(val),
                            })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-col gap-0.5'>
                          <div className='flex items-center gap-1.5'>
                            <span className='text-xs font-medium'>
                              {job.built_in ? t(job.name) : job.name}
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
                          {job.description ? (
                            <span className='text-muted-foreground text-[11px]'>
                              {job.built_in
                                ? t(job.description)
                                : job.description}
                            </span>
                          ) : null}
                          {job.script ? (
                            <span className='text-muted-foreground max-w-xs truncate font-mono text-[11px]'>
                              {job.script}
                            </span>
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
                        <Badge variant='outline' className='font-mono text-xs'>
                          {job.cron_expr}
                        </Badge>
                      </TableCell>
                      <TableCell className='font-mono text-xs'>
                        {job.enabled ? formatUnixTime(job.next_run_at) : '-'}
                      </TableCell>
                      <TableCell className='text-xs'>
                        {job.last_run_at > 0 ? (
                          <div className='flex flex-col gap-0.5'>
                            <div className='flex items-center gap-1.5'>
                              {job.last_status === 'success' ? (
                                <CheckCircle2
                                  aria-hidden='true'
                                  className='size-3.5 text-emerald-500'
                                />
                              ) : null}
                              {job.last_status === 'failed' ? (
                                <XCircle
                                  aria-hidden='true'
                                  className='size-3.5 text-rose-500'
                                />
                              ) : null}
                              {job.last_status !== 'success' &&
                              job.last_status !== 'failed' ? (
                                <RefreshCw
                                  aria-hidden='true'
                                  className='size-3.5 animate-spin text-amber-500'
                                />
                              ) : null}
                              <span className='font-mono'>
                                {formatUnixTime(job.last_run_at)}
                              </span>
                              <span className='text-muted-foreground'>
                                ({job.last_duration_ms}ms)
                              </span>
                            </div>
                            {job.last_output ? (
                              <span className='text-muted-foreground max-w-xs truncate font-mono text-[11px]'>
                                {job.last_output}
                              </span>
                            ) : null}
                          </div>
                        ) : (
                          <span className='text-muted-foreground'>
                            {t('Never executed')}
                          </span>
                        )}
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
                            <span>{t('Run Now')}</span>
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
                            aria-label={`${t('Edit CRON Task')} ${job.built_in ? t(job.name) : job.name}`}
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
                              <Trash2 aria-hidden='true' className='size-3.5' />
                            </Button>
                          ) : null}
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
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
