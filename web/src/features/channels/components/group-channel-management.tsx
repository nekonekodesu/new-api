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
  Activity,
  CheckCircle2,
  Cpu,
  DollarSign,
  Gauge,
  Layers,
  RotateCcw,
  Save,
  Scale,
  Search,
  ShieldCheck,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useMediaQuery } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import {
  type ChannelBrief,
  type CostControlChannelRule,
  type GroupCostControlPolicy,
  type SchedulingMode,
  getGroupScheduling,
  updateGroupScheduling,
} from '../api'
import { CHANNEL_TYPE_NEW_API, CHANNEL_TYPE_OPTIONS } from '../constants'
import {
  DIRECT_UPSTREAM_ID,
  normalizeCostControlPolicy,
  resolveEffectiveChannelPriority,
  resolveEffectiveChannelUpstream,
  resolveEffectiveChannelWeight,
  resolveUpstreamAlloc,
} from '../lib/cost-control'
import { ChannelTypeLogo } from './channel-type-badge'
import { ChannelClassifierPanel } from './channel-classifier-panel'
import {
  CostControlProfitPolicyCard,
  CostControlUpstreamAllocationCard,
} from './cost-control-scheduling-panel'
import { NumericSpinnerInput } from './numeric-spinner-input'

const SCHEDULING_MODES: Array<{
  value: SchedulingMode
  labelKey: string
  descKey: string
  icon: typeof Layers
}> = [
  {
    value: 'standard',
    labelKey: 'Standard Sequential Scheduling',
    descKey:
      'Default New API scheduling by channel priority and weight in sequential order',
    icon: Layers,
  },
  {
    value: 'bbr',
    labelKey: 'BBR Speed & Availability Scheduling',
    descKey:
      'Congestion-aware adaptive routing balancing throughput (TPS), TTFT, and real-time availability',
    icon: Activity,
  },
  {
    value: 'lowercost',
    labelKey: 'Lowest Cost Scheduling (LowerCost)',
    descKey:
      'Routes each model to the channel with the lowest pre-calculated unit cost while guarding availability',
    icon: DollarSign,
  },
  {
    value: 'costcontrol',
    labelKey: 'Cost Control Scheduling (CostControl)',
    descKey:
      'Routes by Upstream Account Allocation and Channel Priority Ratio while tracking real-time net profit and shifting overflow to low-cost channels',
    icon: Scale,
  },
  {
    value: 'speed',
    labelKey: 'Highest Speed Scheduling (Speed)',
    descKey:
      'Prioritizes channels with the highest TPS and lowest first-token latency (TTFT)',
    icon: Gauge,
  },
  {
    value: 'availability',
    labelKey: 'Highest Availability Scheduling (Availability)',
    descKey:
      'Prioritizes channels with the highest historical and EMA success rate',
    icon: ShieldCheck,
  },
]

interface GroupDraftState {
  mode: SchedulingMode
  enabledChannelIds: number[]
  costControl: GroupCostControlPolicy
}

interface ChannelCostControlMeta {
  upstreamId: string
  upstreamName: string
  isDirect: boolean
  isOverridden: boolean
  overrideValue: string
  autoUpstreamName: string
  priority: number
  weight: number
  tierSharePct: number
  isPrimaryTier: boolean
  isOverflowOnly: boolean
  calls: number
  netProfitUSD: number
}

export function GroupChannelManagement(props: {
  initialGroup?: string
  onGroupChange?: (group: string) => void
}) {
  const { t, i18n } = useTranslation()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage ?? i18n.language)
  const queryClient = useQueryClient()

  const [selectedGroup, setSelectedGroup] = useState<string>(
    props.initialGroup || 'default'
  )
  const [searchKeyword, setSearchKeyword] = useState('')
  const [selectedUpstreamFilter, setSelectedUpstreamFilter] =
    useState<string>('all')
  const [draftsByGroup, setDraftsByGroup] = useState<
    Record<string, GroupDraftState>
  >({})

  const schedulingQuery = useQuery({
    queryKey: ['channel-group-scheduling'],
    queryFn: async () => requireServerSuccess(await getGroupScheduling()),
    refetchInterval: 10_000,
  })

  const groups = useMemo(
    () => schedulingQuery.data?.data?.groups ?? [],
    [schedulingQuery.data]
  )
  const channels = useMemo(
    () => schedulingQuery.data?.data?.channels ?? [],
    [schedulingQuery.data]
  )
  const upstreamSources = useMemo(
    () => schedulingQuery.data?.data?.upstream_sources ?? [],
    [schedulingQuery.data]
  )

  const currentGroupSummary = useMemo(
    () =>
      groups.find((item) => item.group === selectedGroup) ??
      groups[0] ??
      undefined,
    [groups, selectedGroup]
  )

  const activeGroup = currentGroupSummary?.group ?? selectedGroup
  const activeDraft = draftsByGroup[activeGroup]
  const draftMode: SchedulingMode =
    activeDraft?.mode ?? currentGroupSummary?.mode ?? 'standard'
  const draftChannelIds: number[] = useMemo(
    () =>
      activeDraft?.enabledChannelIds ??
      currentGroupSummary?.enabled_channel_ids ??
      [],
    [activeDraft?.enabledChannelIds, currentGroupSummary?.enabled_channel_ids]
  )
  const draftCostControl: GroupCostControlPolicy = useMemo(
    () =>
      activeDraft?.costControl ??
      normalizeCostControlPolicy(currentGroupSummary?.cost_control),
    [activeDraft?.costControl, currentGroupSummary?.cost_control]
  )
  const hasUnsavedChanges = Boolean(activeDraft)
  const isCostControlMode = draftMode === 'costcontrol'

  const updateCurrentDraft = (
    updater: (current: GroupDraftState) => GroupDraftState
  ) => {
    setDraftsByGroup((prev) => {
      const base: GroupDraftState = prev[activeGroup] ?? {
        mode: currentGroupSummary?.mode ?? 'standard',
        enabledChannelIds: [...(currentGroupSummary?.enabled_channel_ids ?? [])],
        costControl: normalizeCostControlPolicy(
          currentGroupSummary?.cost_control
        ),
      }
      return {
        ...prev,
        [activeGroup]: updater(base),
      }
    })
  }

  const handleSelectGroup = (groupName: string) => {
    setSelectedGroup(groupName)
    setSelectedUpstreamFilter('all')
    props.onGroupChange?.(groupName)
  }

  const handleUpdatePolicy = (
    updater: (prev: GroupCostControlPolicy) => GroupCostControlPolicy
  ) => {
    updateCurrentDraft((cur) => ({
      ...cur,
      costControl: updater(cur.costControl),
    }))
  }

  const updateChannelRule = (
    ch: ChannelBrief,
    patch: Partial<CostControlChannelRule>
  ) => {
    handleUpdatePolicy((prev) => {
      const key = String(ch.id)
      const existing = prev.channel_rules[key] ?? {}
      const merged: CostControlChannelRule = {
        ...existing,
        ...patch,
      }
      if (!merged.upstream_id) {
        delete merged.upstream_id
      }
      return {
        ...prev,
        channel_rules: {
          ...prev.channel_rules,
          [key]: merged,
        },
      }
    })
  }

  const handleResetChannelRules = () => {
    handleUpdatePolicy((prev) => ({
      ...prev,
      channel_rules: {},
    }))
  }

  const saveMutation = useMutation({
    mutationFn: async () =>
      requireServerSuccess(
        await updateGroupScheduling({
          group: activeGroup,
          mode: draftMode,
          enabled_channel_ids: draftChannelIds,
          cost_control: draftCostControl,
        })
      ),
    onSuccess: () => {
      setDraftsByGroup((prev) => {
        const next = { ...prev }
        delete next[activeGroup]
        return next
      })
      toast.success(t('Group channel and scheduling settings saved'))
      void queryClient.invalidateQueries({
        queryKey: ['channel-group-scheduling'],
      })
      void queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save group scheduling settings'))
    },
  })

  const enabledSet = useMemo(() => new Set(draftChannelIds), [draftChannelIds])

  const channelMetaMap = useMemo(() => {
    const topPriorityByUpstream = new Map<string, number>()
    const tierWeightSum = new Map<string, number>()

    const tempItems = channels.map((ch) => {
      const resolved = resolveEffectiveChannelUpstream(
        ch,
        draftCostControl,
        upstreamSources
      )
      const upAlloc = resolveUpstreamAlloc(resolved.id, draftCostControl)
      const priority = resolveEffectiveChannelPriority(ch, draftCostControl)
      const weight = resolveEffectiveChannelWeight(ch, draftCostControl)
      const isEnabledInGroup = enabledSet.has(ch.id)
      const isActivePreferred =
        isEnabledInGroup && upAlloc.enabled && upAlloc.weight > 0 && weight > 0

      if (isActivePreferred) {
        const curTop = topPriorityByUpstream.get(resolved.id)
        if (curTop === undefined || priority > curTop) {
          topPriorityByUpstream.set(resolved.id, priority)
        }
        const tierKey = `${resolved.id}::${priority}`
        tierWeightSum.set(tierKey, (tierWeightSum.get(tierKey) ?? 0) + weight)
      }

      return {
        ch,
        resolved,
        upAlloc,
        priority,
        weight,
        isEnabledInGroup,
        isActivePreferred,
      }
    })

    const out = new Map<number, ChannelCostControlMeta>()
    for (const item of tempItems) {
      const topPrio = topPriorityByUpstream.get(item.resolved.id)
      const tierKey = `${item.resolved.id}::${item.priority}`
      const sumW = tierWeightSum.get(tierKey) ?? 0
      const tierSharePct =
        item.isActivePreferred && sumW > 0 ? (item.weight / sumW) * 100 : 0
      const isPrimaryTier =
        item.isActivePreferred &&
        topPrio !== undefined &&
        item.priority === topPrio
      const isOverflowOnly =
        item.isEnabledInGroup && !item.isActivePreferred

      const autoId = item.ch.upstream_source_id?.trim() || DIRECT_UPSTREAM_ID
      let autoName = item.ch.upstream_source_name || 'Direct'
      if (autoId === DIRECT_UPSTREAM_ID) {
        autoName = t('Direct / Independent Channels')
      } else {
        const matchedSrc = upstreamSources.find((s) => s.id === autoId)
        if (matchedSrc) {
          autoName = matchedSrc.name
        }
      }

      const rule = draftCostControl.channel_rules[String(item.ch.id)]
      const profitStat =
        currentGroupSummary?.realtime_profit?.by_channel?.[String(item.ch.id)]

      out.set(item.ch.id, {
        upstreamId: item.resolved.id,
        upstreamName: item.resolved.isDirect
          ? t('Direct / Independent Channels')
          : item.resolved.name,
        isDirect: item.resolved.isDirect,
        isOverridden: item.resolved.isOverridden,
        overrideValue: rule?.upstream_id ?? '',
        autoUpstreamName: autoName,
        priority: item.priority,
        weight: item.weight,
        tierSharePct,
        isPrimaryTier,
        isOverflowOnly,
        calls: profitStat?.calls ?? 0,
        netProfitUSD: profitStat?.net_profit_usd ?? 0,
      })
    }
    return out
  }, [
    channels,
    draftCostControl,
    upstreamSources,
    enabledSet,
    currentGroupSummary?.realtime_profit,
    t,
  ])

  const upstreamFilterCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const ch of channels) {
      const meta = channelMetaMap.get(ch.id)
      if (meta) {
        counts.set(meta.upstreamId, (counts.get(meta.upstreamId) ?? 0) + 1)
      }
    }
    return counts
  }, [channels, channelMetaMap])

  const filteredChannels = useMemo(() => {
    const kw = searchKeyword.trim().toLowerCase()
    return channels.filter((ch) => {
      if (
        isCostControlMode &&
        selectedUpstreamFilter !== 'all' &&
        channelMetaMap.get(ch.id)?.upstreamId !== selectedUpstreamFilter
      ) {
        return false
      }
      if (!kw) return true
      const meta = channelMetaMap.get(ch.id)
      return (
        ch.name.toLowerCase().includes(kw) ||
        ch.models.toLowerCase().includes(kw) ||
        ch.tag.toLowerCase().includes(kw) ||
        String(ch.id).includes(kw) ||
        Boolean(meta?.upstreamName.toLowerCase().includes(kw))
      )
    })
  }, [
    channels,
    searchKeyword,
    isCostControlMode,
    selectedUpstreamFilter,
    channelMetaMap,
  ])

  const toggleChannel = (channelId: number, checked: boolean) => {
    updateCurrentDraft((cur) => {
      if (checked) {
        const nextIds = cur.enabledChannelIds.includes(channelId)
          ? cur.enabledChannelIds
          : [...cur.enabledChannelIds, channelId]
        return { ...cur, enabledChannelIds: nextIds }
      }
      return {
        ...cur,
        enabledChannelIds: cur.enabledChannelIds.filter(
          (id) => id !== channelId
        ),
      }
    })
  }

  const handleToggleAllFiltered = (enableAll: boolean) => {
    const filteredIds = filteredChannels.map((c) => c.id)
    updateCurrentDraft((cur) => {
      const set = new Set(cur.enabledChannelIds)
      for (const id of filteredIds) {
        if (enableAll) {
          set.add(id)
        } else {
          set.delete(id)
        }
      }
      return { ...cur, enabledChannelIds: [...set] }
    })
  }

  const formatMoney = (usdVal: number) =>
    formatBillingCurrencyFromUSD(usdVal, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
      locale: intlLocale,
    })

  const renderTierShareBadge = (
    isChecked: boolean,
    meta: ChannelCostControlMeta
  ) => {
    if (!isChecked) {
      return (
        <span className='text-muted-foreground text-[11px]'>
          {t('Not in group')}
        </span>
      )
    }
    if (meta.isOverflowOnly) {
      return (
        <Badge variant='outline' className='px-1.5 py-0 text-[10px]'>
          {t('Overflow Only')}
        </Badge>
      )
    }
    if (meta.isPrimaryTier) {
      return (
        <Badge
          variant='secondary'
          className='border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 font-mono text-[10px] text-emerald-700 dark:text-emerald-300'
        >
          {t('Primary')} · {meta.tierSharePct.toFixed(0)}%
        </Badge>
      )
    }
    return (
      <Badge variant='outline' className='px-1.5 py-0 font-mono text-[10px]'>
        {t('Standby')} · {meta.tierSharePct.toFixed(0)}%
      </Badge>
    )
  }

  const getChannelTypeLabel = (type: number) =>
    CHANNEL_TYPE_OPTIONS.find((item) => item.value === type)?.label ??
    `#${type}`
  const isMobile = useMediaQuery('(max-width: 640px)')
  const hasChannelRuleOverrides =
    Object.keys(draftCostControl.channel_rules).length > 0

  if (schedulingQuery.isLoading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-14 w-full rounded-xl' />
        <Skeleton className='h-44 w-full rounded-xl' />
        <Skeleton className='h-80 w-full rounded-xl' />
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      {/* Group Sub-tabs */}
      <div className='border-border/60 bg-muted/20 flex flex-wrap items-center justify-between gap-3 rounded-xl border p-3'>
        <div className='flex flex-wrap items-center gap-2'>
          {groups.map((g) => {
            const isActive = g.group === activeGroup
            const grpDraft = draftsByGroup[g.group]
            const enabledCount =
              grpDraft?.enabledChannelIds.length ?? g.enabled_channel_ids.length
            return (
              <button
                key={g.group}
                type='button'
                onClick={() => handleSelectGroup(g.group)}
                className={cn(
                  'flex items-center gap-2 rounded-lg border px-3.5 py-2 text-xs font-medium transition-all',
                  isActive
                    ? 'border-primary bg-primary text-primary-foreground shadow-xs'
                    : 'border-border/60 bg-background text-foreground hover:bg-muted/60'
                )}
              >
                <span className='font-semibold'>{g.group}</span>
                <Badge
                  variant={isActive ? 'secondary' : 'outline'}
                  className='px-1.5 py-0 text-[10px]'
                >
                  {g.ratio}x
                </Badge>
                <span className='opacity-75'>({enabledCount})</span>
              </button>
            )
          })}
        </div>

        <div className='flex w-full flex-wrap items-center justify-end gap-2 sm:w-auto'>
          {hasUnsavedChanges && (
            <Badge variant='outline' className='text-[11px]'>
              {t('Unsaved changes')}
            </Badge>
          )}
          <Button
            size='sm'
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || !currentGroupSummary}
            className='w-full gap-1.5 sm:w-auto'
          >
            <Save aria-hidden='true' className='size-4' />
            <span>{t('Save Group Settings')}</span>
          </Button>
        </div>
      </div>

      {/* Group Scheduling Mode Selector */}
      <div className='border-border/60 bg-card rounded-xl border p-4'>
        <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
          <div>
            <h3 className='text-sm font-semibold'>
              {t('Group Scheduling Strategy')} —{' '}
              <span className='text-primary font-mono'>{activeGroup}</span>
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Configure how upstream channels are scheduled for requests in this user group'
              )}
            </p>
          </div>
          {currentGroupSummary?.description && (
            <Badge variant='outline' className='text-xs'>
              {currentGroupSummary.description}
            </Badge>
          )}
        </div>

        <div className='grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6'>
          {SCHEDULING_MODES.map((item) => {
            const Icon = item.icon
            const selected = draftMode === item.value
            return (
              <button
                key={item.value}
                type='button'
                onClick={() =>
                  updateCurrentDraft((cur) => ({
                    ...cur,
                    mode: item.value,
                  }))
                }
                className={cn(
                  'flex flex-col items-start gap-1.5 rounded-xl border p-3 text-left transition-all',
                  selected
                    ? 'border-primary bg-primary/5 ring-primary/20 ring-2'
                    : 'border-border/60 bg-background hover:bg-muted/40'
                )}
              >
                <div className='flex w-full items-center justify-between'>
                  <Icon
                    aria-hidden='true'
                    className={cn(
                      'size-4',
                      selected ? 'text-primary' : 'text-muted-foreground'
                    )}
                  />
                  {selected && (
                    <CheckCircle2
                      aria-hidden='true'
                      className='text-primary size-4'
                    />
                  )}
                </div>
                <span className='text-xs font-semibold'>
                  {t(item.labelKey)}
                </span>
                <span className='text-muted-foreground line-clamp-3 text-[11px] leading-snug'>
                  {t(item.descKey)}
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {/* Cost Control Panels: Real-time Profit + Upstream Account Allocation */}
      {isCostControlMode && (
        <>
          <CostControlProfitPolicyCard
            group={activeGroup}
            groupRatio={currentGroupSummary?.ratio ?? 1}
            policy={draftCostControl}
            snapshot={currentGroupSummary?.realtime_profit}
            isRefreshing={schedulingQuery.isFetching}
            onRefresh={() => {
              void schedulingQuery.refetch()
            }}
            onChangePolicy={handleUpdatePolicy}
          />

          <CostControlUpstreamAllocationCard
            group={activeGroup}
            policy={draftCostControl}
            upstreamSources={upstreamSources}
            channels={channels}
            enabledChannelIds={draftChannelIds}
            snapshot={currentGroupSummary?.realtime_profit}
            selectedUpstreamFilter={selectedUpstreamFilter}
            onSelectUpstreamFilter={setSelectedUpstreamFilter}
            onChangePolicy={handleUpdatePolicy}
          />
        </>
      )}

      {/* Channels Assignment Table for Selected Group */}
      <div className='border-border/60 bg-card flex flex-col gap-3 rounded-xl border p-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Cpu aria-hidden='true' className='text-primary size-4' />
            <h3 className='text-sm font-semibold'>
              {t('Assigned Channels for Group')} ({draftChannelIds.length} /{' '}
              {channels.length})
            </h3>
            {isCostControlMode && (
              <Badge variant='secondary' className='text-[11px]'>
                {t('Channel Priority & Allocation Ratio')}
              </Badge>
            )}
          </div>
          <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
            <div className='relative w-full sm:w-60'>
              <Search
                aria-hidden='true'
                className='text-muted-foreground absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2'
              />
              <Input
                value={searchKeyword}
                onChange={(e) => setSearchKeyword(e.target.value)}
                placeholder={t('Filter channels or models...')}
                className='h-8 ps-8 text-xs'
              />
            </div>
            {isCostControlMode && hasChannelRuleOverrides && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='flex-1 gap-1 sm:flex-initial'
                onClick={handleResetChannelRules}
              >
                <RotateCcw aria-hidden='true' className='size-3.5' />
                <span>{t('Reset Ratios')}</span>
              </Button>
            )}
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='flex-1 sm:flex-initial'
              onClick={() => handleToggleAllFiltered(true)}
            >
              {t('Enable All Shown')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='flex-1 sm:flex-initial'
              onClick={() => handleToggleAllFiltered(false)}
            >
              {t('Disable All Shown')}
            </Button>
          </div>
        </div>

        {/* Upstream Quick Filter Bar in Cost Control Mode */}
        {isCostControlMode && upstreamSources.length > 0 && (
          <div className='flex flex-wrap items-center gap-1.5'>
            <button
              type='button'
              onClick={() => setSelectedUpstreamFilter('all')}
              className={cn(
                'rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors',
                selectedUpstreamFilter === 'all'
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border/60 bg-background hover:bg-muted/60'
              )}
            >
              {t('All Upstreams')} ({channels.length})
            </button>
            {upstreamSources.map((src) => {
              const label = src.is_direct
                ? t('Direct / Independent Channels')
                : src.name
              const count = upstreamFilterCounts.get(src.id) ?? 0
              const active = selectedUpstreamFilter === src.id
              return (
                <button
                  key={src.id}
                  type='button'
                  onClick={() =>
                    setSelectedUpstreamFilter(active ? 'all' : src.id)
                  }
                  className={cn(
                    'rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors',
                    active
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-border/60 bg-background hover:bg-muted/60'
                  )}
                >
                  {label} ({count})
                </button>
              )
            })}
          </div>
        )}

        {isMobile ? (
          <div className='flex flex-col gap-2.5'>
            {filteredChannels.length === 0 ? (
              <div className='text-muted-foreground py-8 text-center text-sm'>
                {t('No matching channels found')}
              </div>
            ) : (
              filteredChannels.map((ch) => {
                const isChecked = enabledSet.has(ch.id)
                const modelList = ch.models
                  ? ch.models.split(',').filter(Boolean)
                  : []
                const meta = channelMetaMap.get(ch.id)
                let chProfitClass = 'text-muted-foreground'
                if (meta && meta.netProfitUSD > 1e-9) {
                  chProfitClass = 'text-emerald-600 dark:text-emerald-400'
                } else if (meta && meta.netProfitUSD < -1e-9) {
                  chProfitClass = 'text-destructive'
                }

                return (
                  <div
                    key={ch.id}
                    className={cn(
                      'border-border/60 flex flex-col gap-2.5 rounded-xl border p-3 text-xs transition-colors',
                      isChecked
                        ? 'border-primary/40 bg-primary/[0.03]'
                        : 'bg-background'
                    )}
                  >
                    <div className='flex items-start justify-between gap-2'>
                      <label className='flex min-w-0 flex-1 cursor-pointer items-center gap-2.5'>
                        <Checkbox
                          checked={isChecked}
                          onCheckedChange={(val) =>
                            toggleChannel(ch.id, Boolean(val))
                          }
                          aria-label={`${t('Enable channel')} ${ch.name}`}
                        />
                        <div className='min-w-0'>
                          <div className='flex flex-wrap items-center gap-1.5'>
                            <span className='font-semibold break-all'>
                              {ch.name}
                            </span>
                            <span className='text-muted-foreground font-mono text-[11px]'>
                              #{ch.id}
                            </span>
                            {ch.type === CHANNEL_TYPE_NEW_API && (
                              <Badge variant='secondary' className='text-[10px]'>
                                New API
                              </Badge>
                            )}
                          </div>
                          <div className='text-muted-foreground mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px]'>
                            <ChannelTypeLogo type={ch.type} size={13} />
                            <span>{t(getChannelTypeLabel(ch.type))}</span>
                            {typeof ch.base_multiplier === 'number' &&
                              ch.base_multiplier > 0 && (
                                <>
                                  <span>·</span>
                                  <span className='font-mono'>
                                    {ch.base_multiplier}x {ch.currency || 'USD'}
                                  </span>
                                </>
                              )}
                          </div>
                        </div>
                      </label>
                      {isCostControlMode &&
                        meta &&
                        renderTierShareBadge(isChecked, meta)}
                    </div>

                    {isCostControlMode && meta ? (
                      <div className='border-border/50 bg-muted/25 flex flex-col gap-2.5 rounded-lg border p-2.5'>
                        <div className='flex flex-wrap items-center justify-between gap-2'>
                          <div className='flex min-w-0 flex-1 flex-col gap-1'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Upstream Account')}
                            </span>
                            <select
                              aria-label={`${t('Upstream Account')} ${ch.name}`}
                              value={meta.overrideValue}
                              onChange={(e) =>
                                updateChannelRule(ch, {
                                  upstream_id: e.target.value || undefined,
                                })
                              }
                              className='border-input bg-background focus-visible:ring-ring/50 h-7 w-full rounded-md border px-2 text-xs outline-none focus-visible:ring-2'
                            >
                              <option value=''>
                                {t('Auto')} ({meta.autoUpstreamName})
                              </option>
                              {upstreamSources.map((src) => (
                                <option key={src.id} value={src.id}>
                                  {src.is_direct
                                    ? t('Direct / Independent Channels')
                                    : src.name}
                                </option>
                              ))}
                            </select>
                          </div>

                          <div className='flex flex-col items-end gap-0.5'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Calls / Net Profit')}
                            </span>
                            <span
                              className={cn(
                                'font-mono text-[11px] font-semibold',
                                chProfitClass
                              )}
                            >
                              {formatNumber(meta.calls, intlLocale)} ·{' '}
                              {meta.netProfitUSD > 0 ? '+' : ''}
                              {formatMoney(meta.netProfitUSD)}
                            </span>
                          </div>
                        </div>

                        <div className='grid grid-cols-2 gap-2'>
                          <div className='flex flex-col gap-1'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Priority')}
                            </span>
                            <NumericSpinnerInput
                              value={meta.priority}
                              min={-1000}
                              max={10000}
                              step={1}
                              ariaLabel={`${ch.name} ${t('Priority')}`}
                              onChange={(val) =>
                                updateChannelRule(ch, { priority: val })
                              }
                            />
                          </div>
                          <div className='flex flex-col gap-1'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Allocation Ratio')}
                            </span>
                            <NumericSpinnerInput
                              value={meta.weight}
                              min={0}
                              max={1000}
                              step={10}
                              ariaLabel={`${ch.name} ${t('Allocation Ratio')}`}
                              onChange={(val) =>
                                updateChannelRule(ch, { weight: val })
                              }
                            />
                          </div>
                        </div>
                      </div>
                    ) : (
                      <div className='text-muted-foreground font-mono text-[11px]'>
                        P:{ch.priority} / W:{ch.weight}
                      </div>
                    )}

                    <div className='flex flex-wrap items-center gap-1'>
                      {ch.group
                        .split(',')
                        .filter(Boolean)
                        .map((g) => (
                          <Badge
                            key={g}
                            variant={
                              g.trim() === activeGroup ? 'default' : 'outline'
                            }
                            className='px-1.5 py-0 text-[10px]'
                          >
                            {g.trim()}
                          </Badge>
                        ))}
                    </div>

                    <div className='text-muted-foreground text-[11px] break-words'>
                      {modelList.length} {t('models')} (
                      {modelList.slice(0, 3).join(', ')}
                      {modelList.length > 3 ? '...' : ''})
                    </div>
                  </div>
                )
              })
            )}
          </div>
        ) : (
          <div className='border-border/50 max-h-[62dvh] overflow-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-12 text-center'>
                    {t('Enabled')}
                  </TableHead>
                  <TableHead className='w-16'>ID</TableHead>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Type')}</TableHead>
                  {isCostControlMode ? (
                    <>
                      <TableHead>{t('Upstream Account')}</TableHead>
                      <TableHead>{t('Priority & Allocation Ratio')}</TableHead>
                      <TableHead>{t('Tier Share')}</TableHead>
                      <TableHead className='text-end'>
                        {t('Calls / Net Profit')}
                      </TableHead>
                    </>
                  ) : (
                    <>
                      <TableHead>{t('Groups')}</TableHead>
                      <TableHead>{t('Priority / Weight')}</TableHead>
                    </>
                  )}
                  <TableHead>{t('Models')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredChannels.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={isCostControlMode ? 9 : 7}
                      className='text-muted-foreground h-24 text-center text-sm'
                    >
                      {t('No matching channels found')}
                    </TableCell>
                  </TableRow>
                ) : (
                  filteredChannels.map((ch) => {
                    const isChecked = enabledSet.has(ch.id)
                    const modelList = ch.models
                      ? ch.models.split(',').filter(Boolean)
                      : []
                    const meta = channelMetaMap.get(ch.id)
                    let chProfitClass = 'text-muted-foreground'
                    if (meta && meta.netProfitUSD > 1e-9) {
                      chProfitClass = 'text-emerald-600 dark:text-emerald-400'
                    } else if (meta && meta.netProfitUSD < -1e-9) {
                      chProfitClass = 'text-destructive'
                    }

                    return (
                      <TableRow
                        key={ch.id}
                        className={cn(isChecked && 'bg-primary/[0.02]')}
                      >
                        <TableCell className='text-center'>
                          <Checkbox
                            checked={isChecked}
                            onCheckedChange={(val) =>
                              toggleChannel(ch.id, Boolean(val))
                            }
                            aria-label={`${t('Enable channel')} ${ch.name}`}
                          />
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          #{ch.id}
                        </TableCell>
                        <TableCell>
                          <div className='flex flex-col gap-0.5'>
                            <div className='flex flex-wrap items-center gap-2'>
                              <span className='font-medium'>{ch.name}</span>
                              {ch.type === CHANNEL_TYPE_NEW_API && (
                                <Badge
                                  variant='secondary'
                                  className='text-[10px]'
                                >
                                  New API
                                </Badge>
                              )}
                            </div>
                            {isCostControlMode && (
                              <div className='text-muted-foreground flex flex-wrap items-center gap-1.5 text-[11px]'>
                                {typeof ch.base_multiplier === 'number' &&
                                  ch.base_multiplier > 0 && (
                                    <span className='font-mono'>
                                      {ch.base_multiplier}x{' '}
                                      {ch.currency || 'USD'}
                                    </span>
                                  )}
                                {ch.upstream_group && (
                                  <>
                                    <span>·</span>
                                    <span className='font-mono'>
                                      {ch.upstream_group}
                                    </span>
                                  </>
                                )}
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className='flex items-center gap-1.5 text-xs'>
                            <ChannelTypeLogo type={ch.type} size={15} />
                            <span>{t(getChannelTypeLabel(ch.type))}</span>
                          </div>
                        </TableCell>

                        {isCostControlMode && meta ? (
                          <>
                            <TableCell>
                              <select
                                aria-label={`${t('Upstream Account')} ${ch.name}`}
                                value={meta.overrideValue}
                                onChange={(e) =>
                                  updateChannelRule(ch, {
                                    upstream_id: e.target.value || undefined,
                                  })
                                }
                                className='border-input bg-background focus-visible:ring-ring/50 h-7 max-w-44 rounded-md border px-2 text-xs outline-none focus-visible:ring-2'
                              >
                                <option value=''>
                                  {t('Auto')} ({meta.autoUpstreamName})
                                </option>
                                {upstreamSources.map((src) => (
                                  <option key={src.id} value={src.id}>
                                    {src.is_direct
                                      ? t('Direct / Independent Channels')
                                      : src.name}
                                  </option>
                                ))}
                              </select>
                            </TableCell>
                            <TableCell>
                              <div className='flex items-center gap-2'>
                                <div className='flex items-center gap-1'>
                                  <span className='text-muted-foreground font-mono text-[11px]'>
                                    P:
                                  </span>
                                  <NumericSpinnerInput
                                    value={meta.priority}
                                    min={-1000}
                                    max={10000}
                                    step={1}
                                    ariaLabel={`${ch.name} ${t('Priority')}`}
                                    onChange={(val) =>
                                      updateChannelRule(ch, { priority: val })
                                    }
                                  />
                                </div>
                                <div className='flex items-center gap-1'>
                                  <span className='text-muted-foreground font-mono text-[11px]'>
                                    W:
                                  </span>
                                  <NumericSpinnerInput
                                    value={meta.weight}
                                    min={0}
                                    max={1000}
                                    step={10}
                                    ariaLabel={`${ch.name} ${t('Allocation Ratio')}`}
                                    onChange={(val) =>
                                      updateChannelRule(ch, { weight: val })
                                    }
                                  />
                                </div>
                              </div>
                            </TableCell>
                            <TableCell>
                              {renderTierShareBadge(isChecked, meta)}
                            </TableCell>
                            <TableCell className='text-end font-mono text-xs'>
                              <div className='flex flex-col items-end'>
                                <span
                                  className={cn('font-semibold', chProfitClass)}
                                >
                                  {meta.netProfitUSD > 0 ? '+' : ''}
                                  {formatMoney(meta.netProfitUSD)}
                                </span>
                                <span className='text-muted-foreground text-[10px]'>
                                  {formatNumber(meta.calls, intlLocale)}{' '}
                                  {t('Calls')}
                                </span>
                              </div>
                            </TableCell>
                          </>
                        ) : (
                          <>
                            <TableCell>
                              <div className='flex flex-wrap gap-1'>
                                {ch.group
                                  .split(',')
                                  .filter(Boolean)
                                  .map((g) => (
                                    <Badge
                                      key={g}
                                      variant={
                                        g.trim() === activeGroup
                                          ? 'default'
                                          : 'outline'
                                      }
                                      className='px-1.5 py-0 text-[10px]'
                                    >
                                      {g.trim()}
                                    </Badge>
                                  ))}
                              </div>
                            </TableCell>
                            <TableCell className='font-mono text-xs'>
                              P:{ch.priority} / W:{ch.weight}
                            </TableCell>
                          </>
                        )}

                        <TableCell className='max-w-xs truncate text-xs'>
                          {modelList.length} {t('models')} (
                          {modelList.slice(0, 3).join(', ')}
                          {modelList.length > 3 ? '...' : ''})
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      <ChannelClassifierPanel />
    </div>
  )
}
