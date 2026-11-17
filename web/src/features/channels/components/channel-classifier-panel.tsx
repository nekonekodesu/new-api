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
  Edit3,
  Filter,
  Layers,
  Play,
  Plus,
  RefreshCw,
  Tag,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { TagInput } from '@/components/tag-input'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { handleServerError } from '@/lib/handle-server-error'
import {
  createServerError,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import {
  applyChannelClassifierNow,
  type ChannelClassifierRule,
  deleteChannelClassifierRule,
  getChannelClassifier,
  saveChannelClassifier,
} from '../api'

function emptyClassifierRule(): Partial<ChannelClassifierRule> {
  return {
    id: '',
    name: '',
    target_group: '',
    description: '',
    group_ratio: 1.0,
    keywords: [],
    include_native_api: false,
    enabled: true,
  }
}

export function ChannelClassifierPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [editingRule, setEditingRule] =
    useState<Partial<ChannelClassifierRule> | null>(null)
  const [deletingRule, setDeletingRule] =
    useState<ChannelClassifierRule | null>(null)
  const [filterGroup, setFilterGroup] = useState<string>('ALL')

  const classifierQuery = useQuery({
    queryKey: ['channel-classifier'],
    queryFn: async () => requireServerSuccess(await getChannelClassifier()),
  })

  const enabled = classifierQuery.data?.data?.enabled ?? true
  const rules = useMemo(
    () => classifierQuery.data?.data?.rules ?? [],
    [classifierQuery.data?.data?.rules]
  )
  const preview = useMemo(
    () => classifierQuery.data?.data?.preview ?? [],
    [classifierQuery.data?.data?.preview]
  )

  const matchedCountByGroup = useMemo(() => {
    const map: Record<string, number> = {}
    for (const item of preview) {
      for (const g of item.matched_groups ?? []) {
        map[g] = (map[g] ?? 0) + 1
      }
    }
    return map
  }, [preview])

  const filteredPreview = useMemo(() => {
    if (filterGroup === 'ALL') {
      return preview
    }
    if (filterGroup === 'DEFAULT_ONLY') {
      return preview.filter((item) => (item.matched_groups ?? []).length === 0)
    }
    return preview.filter((item) =>
      (item.matched_groups ?? []).includes(filterGroup)
    )
  }, [preview, filterGroup])

  const invalidateRelated = () => {
    void queryClient.invalidateQueries({ queryKey: ['channel-classifier'] })
    void queryClient.invalidateQueries({ queryKey: ['group-scheduling'] })
    void queryClient.invalidateQueries({ queryKey: ['channels'] })
  }

  const saveMutation = useMutation({
    mutationFn: async (payload: {
      enabled?: boolean
      rules?: ChannelClassifierRule[]
      rule?: Partial<ChannelClassifierRule>
      apply_now?: boolean
    }) => {
      const res = await saveChannelClassifier(payload)
      if (!res.success) {
        throw createServerError(res, t('Failed to save classifier rule'))
      }
      return res
    },
    meta: { errorToast: false },
    onSuccess: (res) => {
      const updated = res.data?.summary?.updated_channels ?? 0
      toast.success(
        `${t('Classifier rules saved and applied')} (${t('Updated')}: ${updated})`
      )
      setEditingRule(null)
      invalidateRelated()
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save classifier rule'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await deleteChannelClassifierRule(id)
      if (!res.success) {
        throw createServerError(res, t('Failed to delete classifier rule'))
      }
      return res
    },
    meta: { errorToast: false },
    onSuccess: () => {
      toast.success(t('Classifier rule deleted'))
      setDeletingRule(null)
      invalidateRelated()
    },
    onError: (err) => {
      handleServerError(err, t('Failed to delete classifier rule'))
    },
  })

  const applyMutation = useMutation({
    mutationFn: async () => {
      const res = await applyChannelClassifierNow()
      if (!res.success) {
        throw createServerError(res, t('Failed to apply channel classifier'))
      }
      return res
    },
    meta: { errorToast: false },
    onSuccess: (res) => {
      const summary = res.data?.summary
      toast.success(
        `${t('Channel classification applied')}: ${summary?.updated_channels ?? 0}/${summary?.total_channels ?? 0}`
      )
      invalidateRelated()
    },
    onError: (err) => {
      handleServerError(err, t('Failed to apply channel classifier'))
    },
  })

  const handleSaveRuleDialog = () => {
    if (!editingRule) return
    const targetGroup = (editingRule.target_group ?? '').trim()
    if (!targetGroup) {
      toast.error(t('Target group cannot be empty'))
      return
    }
    if (targetGroup.toLowerCase() === 'default') {
      toast.error(
        t(
          'All channels are already in default group by default; please enter a specific category group name'
        )
      )
      return
    }
    const ratio = Number(editingRule.group_ratio)
    const normalizedRatio = Number.isFinite(ratio) && ratio > 0 ? ratio : 1.0
    const name = (editingRule.name ?? '').trim() || targetGroup
    const description = (editingRule.description ?? '').trim() || name

    saveMutation.mutate({
      rule: {
        ...editingRule,
        name,
        target_group: targetGroup,
        description,
        group_ratio: normalizedRatio,
        keywords: editingRule.keywords ?? [],
        include_native_api: Boolean(editingRule.include_native_api),
        enabled: editingRule.enabled ?? true,
      },
      apply_now: true,
    })
  }

  return (
    <Card className='gap-3 py-3'>
      <CardHeader className='px-4 py-0'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='space-y-0.5'>
            <CardTitle className='flex items-center gap-2 text-sm'>
              <Filter className='text-primary size-4' />
              <span>{t('Upstream Channel Classifier')}</span>
              <Badge variant='secondary' className='text-[11px]'>
                {rules.length} {t('Rules')}
              </Badge>
            </CardTitle>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Automatically split channels into target groups by keywords or native API status. All channels stay in default group so default can draw all channels, while selecting a category group routes only to matching channels and applies its group ratio. Auto-runs with CRON sync.'
              )}
            </p>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <div className='bg-muted/40 flex items-center gap-2 rounded-md border px-2.5 py-1'>
              <Label
                htmlFor='classifier-auto-cron-switch'
                className='cursor-pointer text-xs'
              >
                {t('Sync with CRON')}
              </Label>
              <Switch
                id='classifier-auto-cron-switch'
                checked={enabled}
                disabled={saveMutation.isPending}
                onCheckedChange={(checked) => {
                  saveMutation.mutate({
                    enabled: Boolean(checked),
                    apply_now: Boolean(checked),
                  })
                }}
              />
            </div>
            <Button
              variant='outline'
              size='sm'
              onClick={() => applyMutation.mutate()}
              disabled={applyMutation.isPending || classifierQuery.isFetching}
            >
              <Play
                aria-hidden='true'
                className={cn(
                  'me-1.5 size-3.5',
                  applyMutation.isPending && 'animate-spin'
                )}
              />
              {t('Apply Classifier Now')}
            </Button>
            <Button
              size='sm'
              onClick={() => setEditingRule(emptyClassifierRule())}
            >
              <Plus aria-hidden='true' className='me-1.5 size-3.5' />
              {t('Add Classifier Rule')}
            </Button>
          </div>
        </div>
      </CardHeader>

      <CardContent className='space-y-3 px-4'>
        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow className='text-xs'>
                <TableHead className='w-16'>{t('Enabled')}</TableHead>
                <TableHead>{t('Category Name')}</TableHead>
                <TableHead>{t('Target Group')}</TableHead>
                <TableHead className='w-28'>{t('Group Ratio')}</TableHead>
                <TableHead>{t('Match Keywords')}</TableHead>
                <TableHead className='w-32'>
                  {t('Include Native API')}
                </TableHead>
                <TableHead className='w-28 text-end'>
                  {t('Matched Channels')}
                </TableHead>
                <TableHead className='w-28 text-end'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={8}
                    className='text-muted-foreground py-6 text-center text-xs'
                  >
                    {t('No classifier rules configured yet')}
                  </TableCell>
                </TableRow>
              ) : (
                rules.map((rule) => {
                  const matchedCount =
                    matchedCountByGroup[rule.target_group] ?? 0
                  return (
                    <TableRow key={rule.id} className='text-xs'>
                      <TableCell>
                        <Switch
                          checked={rule.enabled}
                          disabled={saveMutation.isPending}
                          onCheckedChange={(checked) => {
                            saveMutation.mutate({
                              rule: {
                                ...rule,
                                enabled: Boolean(checked),
                              },
                              apply_now: true,
                            })
                          }}
                        />
                      </TableCell>
                      <TableCell>
                        <div className='font-medium'>{rule.name}</div>
                        {rule.description && rule.description !== rule.name ? (
                          <div className='text-muted-foreground text-[11px]'>
                            {rule.description}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <Badge variant='outline' className='font-mono text-xs'>
                          {rule.target_group}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant='secondary'
                          className='font-mono text-xs'
                        >
                          {rule.group_ratio}x
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap gap-1'>
                          {(rule.keywords ?? []).length === 0 ? (
                            <span className='text-muted-foreground text-[11px]'>
                              —
                            </span>
                          ) : (
                            (rule.keywords ?? []).map((kw) => (
                              <Badge
                                key={kw}
                                variant='secondary'
                                className='px-1.5 py-0 text-[10px]'
                              >
                                {kw}
                              </Badge>
                            ))
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        {rule.include_native_api ? (
                          <Badge
                            variant='outline'
                            className='border-emerald-500/40 bg-emerald-500/10 text-[10px] text-emerald-700 dark:text-emerald-300'
                          >
                            <CheckCircle2 className='me-1 size-3' />
                            {t('Included')}
                          </Badge>
                        ) : (
                          <span className='text-muted-foreground text-[11px]'>
                            {t('Keyword Only')}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className='text-end font-mono'>
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-6 px-2 font-mono text-xs'
                          onClick={() =>
                            setFilterGroup(
                              filterGroup === rule.target_group
                                ? 'ALL'
                                : rule.target_group
                            )
                          }
                        >
                          {matchedCount}
                        </Button>
                      </TableCell>
                      <TableCell className='text-end'>
                        <div className='flex items-center justify-end gap-1'>
                          <Button
                            variant='ghost'
                            size='sm'
                            className='h-7 px-2 text-xs'
                            aria-label={`${t('Edit Classifier Rule')} ${rule.name}`}
                            onClick={() =>
                              setEditingRule({
                                ...rule,
                                keywords: [...(rule.keywords ?? [])],
                              })
                            }
                          >
                            <Edit3 aria-hidden='true' className='size-3.5' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='sm'
                            className='text-destructive hover:text-destructive h-7 px-2 text-xs'
                            aria-label={`${t('Delete Classifier Rule')} ${rule.name}`}
                            onClick={() => setDeletingRule(rule)}
                          >
                            <Trash2 aria-hidden='true' className='size-3.5' />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>

        {/* Channel Classification Preview */}
        <div className='space-y-2 rounded-lg border p-3'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div className='flex items-center gap-2'>
              <Layers className='text-muted-foreground size-3.5' />
              <span className='text-xs font-medium'>
                {t('Channel Classification Result & Preview')}
              </span>
              <Badge variant='secondary' className='font-mono text-[10px]'>
                {filteredPreview.length} / {preview.length}
              </Badge>
            </div>
            <div className='flex flex-wrap items-center gap-1'>
              <Button
                variant={filterGroup === 'ALL' ? 'default' : 'outline'}
                size='sm'
                className='h-6 px-2 text-[11px]'
                onClick={() => setFilterGroup('ALL')}
              >
                {t('All')} ({preview.length})
              </Button>
              {rules.map((rule) => (
                <Button
                  key={rule.id}
                  variant={
                    filterGroup === rule.target_group ? 'default' : 'outline'
                  }
                  size='sm'
                  className='h-6 px-2 font-mono text-[11px]'
                  onClick={() => setFilterGroup(rule.target_group)}
                >
                  {rule.target_group} (
                  {matchedCountByGroup[rule.target_group] ?? 0})
                </Button>
              ))}
              <Button
                variant={filterGroup === 'DEFAULT_ONLY' ? 'default' : 'outline'}
                size='sm'
                className='h-6 px-2 text-[11px]'
                onClick={() => setFilterGroup('DEFAULT_ONLY')}
              >
                {t('Default Only')} (
                {
                  preview.filter((p) => (p.matched_groups ?? []).length === 0)
                    .length
                }
                )
              </Button>
              <Button
                variant='ghost'
                size='sm'
                className='h-6 px-2 text-[11px]'
                onClick={() => void classifierQuery.refetch()}
                disabled={classifierQuery.isFetching}
              >
                <RefreshCw
                  aria-hidden='true'
                  className={cn(
                    'size-3',
                    classifierQuery.isFetching && 'animate-spin'
                  )}
                />
              </Button>
            </div>
          </div>

          <div className='max-h-60 overflow-y-auto rounded border'>
            <Table>
              <TableHeader>
                <TableRow className='text-[11px]'>
                  <TableHead className='w-16'>ID</TableHead>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Upstream Group')}</TableHead>
                  <TableHead>{t('API Type')}</TableHead>
                  <TableHead>{t('Matched Categories')}</TableHead>
                  <TableHead>{t('Assigned Local Groups')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredPreview.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className='text-muted-foreground py-4 text-center text-xs'
                    >
                      {t('No matching channels')}
                    </TableCell>
                  </TableRow>
                ) : (
                  filteredPreview.map((item) => (
                    <TableRow key={item.channel_id} className='text-xs'>
                      <TableCell className='font-mono text-[11px]'>
                        #{item.channel_id}
                      </TableCell>
                      <TableCell className='font-medium'>
                        {item.channel_name}
                      </TableCell>
                      <TableCell className='font-mono text-[11px]'>
                        {item.upstream_group || '—'}
                      </TableCell>
                      <TableCell>
                        {item.is_native_api ? (
                          <Badge
                            variant='outline'
                            className='border-blue-500/40 bg-blue-500/10 text-[10px] text-blue-700 dark:text-blue-300'
                          >
                            {t('Native API')}
                          </Badge>
                        ) : (
                          <span className='text-muted-foreground text-[11px]'>
                            NewAPI
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap gap-1'>
                          {(item.matched_groups ?? []).length === 0 ? (
                            <span className='text-muted-foreground text-[11px]'>
                              —
                            </span>
                          ) : (
                            (item.matched_groups ?? []).map((g) => (
                              <Badge
                                key={g}
                                variant='default'
                                className='px-1.5 py-0 font-mono text-[10px]'
                              >
                                <Tag className='me-1 size-2.5' />
                                {g}
                              </Badge>
                            ))
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap gap-1'>
                          {item.final_groups
                            .split(',')
                            .map((g) => g.trim())
                            .filter(Boolean)
                            .map((g) => (
                              <Badge
                                key={g}
                                variant={
                                  g === 'default' ? 'secondary' : 'outline'
                                }
                                className='px-1.5 py-0 font-mono text-[10px]'
                              >
                                {g}
                              </Badge>
                            ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </div>
      </CardContent>

      <Dialog
        open={Boolean(editingRule)}
        onOpenChange={(open) => {
          if (!open) setEditingRule(null)
        }}
        title={
          editingRule?.id
            ? t('Edit Classifier Rule')
            : t('Add Classifier Rule')
        }
        description={t(
          'Configure keyword matching, native API inclusion, target group name, and group billing ratio.'
        )}
        contentClassName='sm:max-w-lg'
        footer={
          <>
            <Button variant='outline' onClick={() => setEditingRule(null)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={handleSaveRuleDialog}
              disabled={saveMutation.isPending}
            >
              {t('Save & Apply')}
            </Button>
          </>
        }
      >
        {editingRule ? (
          <div className='space-y-4'>
            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='space-y-1.5'>
                <Label htmlFor='classifier-rule-name-input'>
                  {t('Category Name')}
                </Label>
                <Input
                  id='classifier-rule-name-input'
                  value={editingRule.name ?? ''}
                  onChange={(e) =>
                    setEditingRule({
                      ...editingRule,
                      name: e.target.value,
                    })
                  }
                  placeholder={t('e.g. 乖乖分組 (CTF)')}
                />
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='classifier-rule-target-group-input'>
                  {t('Target Group')}
                </Label>
                <Input
                  id='classifier-rule-target-group-input'
                  value={editingRule.target_group ?? ''}
                  onChange={(e) =>
                    setEditingRule({
                      ...editingRule,
                      target_group: e.target.value,
                    })
                  }
                  placeholder={t('e.g. ctf / official / full')}
                  className='font-mono'
                />
              </div>
            </div>

            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='space-y-1.5'>
                <Label htmlFor='classifier-rule-group-ratio-input'>
                  {t('Group Ratio')}
                </Label>
                <Input
                  id='classifier-rule-group-ratio-input'
                  type='number'
                  step='0.05'
                  min='0.01'
                  value={editingRule.group_ratio ?? ''}
                  onChange={(e) => {
                    const raw = e.target.value
                    setEditingRule({
                      ...editingRule,
                      group_ratio:
                        raw === '' ? (undefined as unknown as number) : Number(raw),
                    })
                  }}
                  className='font-mono'
                />
                <p className='text-muted-foreground text-[11px]'>
                  {t(
                    'Synced to system GroupRatio & UserUsableGroups so tokens/users selecting this group use this multiplier.'
                  )}
                </p>
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='classifier-rule-description-input'>
                  {t('Group Description')}
                </Label>
                <Input
                  id='classifier-rule-description-input'
                  value={editingRule.description ?? ''}
                  onChange={(e) =>
                    setEditingRule({
                      ...editingRule,
                      description: e.target.value,
                    })
                  }
                  placeholder={t('Shown to users when selecting a group')}
                />
              </div>
            </div>

            <div className='space-y-1.5'>
              <Label>{t('Match Keywords')}</Label>
              <TagInput
                value={editingRule.keywords ?? []}
                onChange={(keywords) =>
                  setEditingRule({
                    ...editingRule,
                    keywords,
                  })
                }
                placeholder={t(
                  'Type keyword and press Enter or comma (e.g. 乖乖, 官, 不降智, 滿智)'
                )}
              />
              <p className='text-muted-foreground text-[11px]'>
                {t(
                  'Matches against channel name, tag, or upstream group name (case-insensitive).'
                )}
              </p>
            </div>

            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='flex items-center justify-between rounded-lg border p-3'>
                <div className='space-y-0.5 pe-2'>
                  <Label
                    htmlFor='classifier-rule-native-api-switch'
                    className='cursor-pointer text-xs font-medium'
                  >
                    {t('Include Native API Channels')}
                  </Label>
                  <p className='text-muted-foreground text-[11px]'>
                    {t(
                      'Automatically include all direct/native provider API channels in this group'
                    )}
                  </p>
                </div>
                <Switch
                  id='classifier-rule-native-api-switch'
                  checked={Boolean(editingRule.include_native_api)}
                  onCheckedChange={(checked) =>
                    setEditingRule({
                      ...editingRule,
                      include_native_api: Boolean(checked),
                    })
                  }
                />
              </div>

              <div className='flex items-center justify-between rounded-lg border p-3'>
                <div className='space-y-0.5 pe-2'>
                  <Label
                    htmlFor='classifier-rule-enabled-switch'
                    className='cursor-pointer text-xs font-medium'
                  >
                    {t('Rule Enabled')}
                  </Label>
                  <p className='text-muted-foreground text-[11px]'>
                    {t('Apply this rule during classification and CRON sync')}
                  </p>
                </div>
                <Switch
                  id='classifier-rule-enabled-switch'
                  checked={editingRule.enabled ?? true}
                  onCheckedChange={(checked) =>
                    setEditingRule({
                      ...editingRule,
                      enabled: Boolean(checked),
                    })
                  }
                />
              </div>
            </div>
          </div>
        ) : null}
      </Dialog>

      <ConfirmDialog
        open={Boolean(deletingRule)}
        onOpenChange={(open) => {
          if (!open) setDeletingRule(null)
        }}
        title={t('Delete Classifier Rule')}
        desc={t(
          'Are you sure you want to delete this classifier rule? Channels will be re-evaluated without this rule.'
        )}
        confirmText={t('Delete')}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deletingRule) {
            deleteMutation.mutate(deletingRule.id)
          }
        }}
      />
    </Card>
  )
}
