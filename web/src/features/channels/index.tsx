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
import { useQuery } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import {
  Clock,
  Cpu,
  DollarSign,
  Layers,
  ListFilter,
  Radio,
  RefreshCw,
  Server,
  Settings2,
  Users,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps } from './api'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'
import { CostManagementPanel } from './components/cost-management-panel'
import { ChannelClassifierPanel } from './components/channel-classifier-panel'
import { CronJobsPanel } from './components/cron-jobs-panel'
import { CronUpstreamSyncPanel } from './components/cron-upstream-sync-panel'
import { GroupChannelManagement } from './components/group-channel-management'
import { ModelPriceTablePanel } from './components/model-price-table-panel'
import { UpstreamAccountSyncPanel } from './components/upstream-account-sync-panel'

type ChannelSubTab =
  | 'all'
  | 'groups'
  | 'price-table'
  | 'classifier'
  | 'cron'
  | 'upstream-accounts'
  | 'upstream-sync'
  | 'cost-models'
  | 'cost-users'

function normalizeChannelSubTab(raw: string | undefined): ChannelSubTab {
  if (raw === 'cron-sync' || raw === 'upstream-sync') {
    return 'upstream-sync'
  }
  if (
    raw === 'groups' ||
    raw === 'price-table' ||
    raw === 'classifier' ||
    raw === 'cron' ||
    raw === 'upstream-accounts' ||
    raw === 'cost-models' ||
    raw === 'cost-users'
  ) {
    return raw
  }
  return 'all'
}

export function Channels() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const searchParams = new URLSearchParams(location.searchStr || '')
  const urlTab = normalizeChannelSubTab(searchParams.get('tab') ?? undefined)
  const [activeTab, setActiveTab] = useState<ChannelSubTab>(urlTab)

  useEffect(() => {
    setActiveTab(urlTab)
  }, [urlTab])

  const handleTabChange = (nextTab: string) => {
    const normalized = normalizeChannelSubTab(nextTab)
    setActiveTab(normalized)
    void navigate({
      to: '/channels',
      search: (prev: Record<string, unknown>) => ({
        ...prev,
        tab: normalized === 'all' ? undefined : normalized,
      }),
    })
  }

  const isCronSection =
    activeTab === 'cron' ||
    activeTab === 'upstream-accounts' ||
    activeTab === 'upstream-sync'
  const isCostSection =
    activeTab === 'cost-models' || activeTab === 'cost-users'

  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const channelOpsQuery = useQuery({
    queryKey: ['channel-ops'],
    queryFn: async () => requireServerSuccess(await getChannelOps()),
    retry: false,
    staleTime: 5 * 60 * 1000,
    enabled: !isCronSection && !isCostSection,
  })
  const retryTimes = channelOpsQuery.data?.data?.retry_times
  const retryLabel =
    typeof retryTimes === 'number' ? `${t('Max Retries')}: ${retryTimes}` : null
  let retryBadge = null
  if (retryLabel && !isCronSection && !isCostSection) {
    retryBadge = isRoot ? (
      <Tooltip>
        <TooltipTrigger
          render={
            <Badge
              variant='outline'
              className='shrink-0 cursor-pointer'
              aria-label={t('Retry Settings')}
              render={
                <Link
                  to='/system-settings/request-policies/$section'
                  params={{ section: 'routing' }}
                />
              }
            />
          }
        >
          <span>{retryLabel}</span>
          <Settings2 data-icon='inline-end' />
        </TooltipTrigger>
        <TooltipContent>
          <p>{t('Retry Settings')}</p>
        </TooltipContent>
      </Tooltip>
    ) : (
      <Badge variant='outline' className='shrink-0'>
        {retryLabel}
      </Badge>
    )
  }

  let sectionTitle = t('Channels')
  if (isCronSection) {
    sectionTitle = t('Scheduled Tasks')
  } else if (isCostSection) {
    sectionTitle = t('Cost Management')
  }

  return (
    <ChannelsProvider>
      <SectionPageLayout
        fixedContent={activeTab === 'all'}
        stackActionsOnMobile={activeTab === 'all'}
      >
        <SectionPageLayout.Title>
          <span className='flex min-w-0 items-center gap-2'>
            <span className='truncate'>{sectionTitle}</span>
            {retryBadge}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {activeTab === 'all' ? <ChannelsPrimaryButtons /> : null}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs
            value={activeTab}
            onValueChange={(value) => handleTabChange(String(value))}
            className='mb-3 w-full shrink-0'
          >
            {isCronSection ? (
              <div className='max-w-full overflow-x-auto pb-0.5'>
                <TabsList className='inline-flex h-auto min-w-full justify-start gap-1 p-1 sm:min-w-0 sm:flex-wrap'>
                  <TabsTrigger
                    value='cron'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <Clock aria-hidden='true' className='size-3.5 shrink-0' />
                    <span>{t('CRON Scheduled Tasks')}</span>
                  </TabsTrigger>
                  <TabsTrigger
                    value='upstream-accounts'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <Server aria-hidden='true' className='size-3.5 shrink-0' />
                    <span>{t('Upstream Account & Group Channel Sync')}</span>
                  </TabsTrigger>
                  <TabsTrigger
                    value='upstream-sync'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <RefreshCw
                      aria-hidden='true'
                      className='size-3.5 shrink-0'
                    />
                    <span>{t('Channel Upstream Model Sync')}</span>
                  </TabsTrigger>
                </TabsList>
              </div>
            ) : null}

            {isCostSection ? (
              <div className='max-w-full overflow-x-auto pb-0.5'>
                <TabsList className='inline-flex h-auto min-w-full justify-start gap-1 p-1 sm:min-w-0 sm:flex-wrap'>
                  <TabsTrigger
                    value='cost-models'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <Cpu aria-hidden='true' className='size-3.5 shrink-0' />
                    <span>{t('Model Cost & Selling Price')}</span>
                  </TabsTrigger>
                  <TabsTrigger
                    value='cost-users'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <Users aria-hidden='true' className='size-3.5 shrink-0' />
                    <span>{t('User Top-up & Gross Margin')}</span>
                  </TabsTrigger>
                </TabsList>
              </div>
            ) : null}

            {!isCronSection && !isCostSection ? (
              <div className='max-w-full overflow-x-auto pb-0.5'>
                <TabsList className='inline-flex h-auto min-w-full justify-start gap-1 p-1 sm:min-w-0 sm:flex-wrap'>
                  <TabsTrigger value='all' className='shrink-0 gap-1.5 text-xs'>
                    <Radio aria-hidden='true' className='size-3.5 shrink-0' />
                    <span>{t('All Channels')}</span>
                  </TabsTrigger>
                  <TabsTrigger
                    value='groups'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <Layers aria-hidden='true' className='size-3.5 shrink-0' />
                    <span>{t('Group Channel Management')}</span>
                  </TabsTrigger>
                   <TabsTrigger
                     value='price-table'
                    className='shrink-0 gap-1.5 text-xs'
                  >
                    <DollarSign
                      aria-hidden='true'
                      className='size-3.5 shrink-0'
                    />
                    <span>{t('Model Price Table')}</span>
                   </TabsTrigger>
                   <TabsTrigger
                     value='classifier'
                     className='shrink-0 gap-1.5 text-xs'
                   >
                     <ListFilter aria-hidden='true' className='size-3.5 shrink-0' />
                     <span>{t('Channel Classifier')}</span>
                   </TabsTrigger>
                </TabsList>
              </div>
            ) : null}
          </Tabs>

          <div className='flex min-h-0 flex-1 flex-col'>
            {activeTab === 'all' && <ChannelsTable />}
            {activeTab === 'groups' && <GroupChannelManagement />}
            {activeTab === 'price-table' && <ModelPriceTablePanel />}
            {activeTab === 'classifier' && <ChannelClassifierPanel />}
            {activeTab === 'cron' && <CronJobsPanel />}
            {activeTab === 'upstream-accounts' && <UpstreamAccountSyncPanel />}
            {activeTab === 'upstream-sync' && <CronUpstreamSyncPanel />}
            {activeTab === 'cost-models' && (
              <CostManagementPanel mode='models' />
            )}
            {activeTab === 'cost-users' && <CostManagementPanel mode='users' />}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
