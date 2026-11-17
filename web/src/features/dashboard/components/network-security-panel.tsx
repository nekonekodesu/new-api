import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { api } from '@/lib/api'
import { formatDateTimeStr } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

type SecurityBan = {
  ip: string
  reason: string
  banned_at: string
  expires_at: string
}

const NETWORK_SECURITY_BANS_QUERY_KEY = [
  'dashboard',
  'network-security',
  'bans',
] as const

async function getNetworkSecurityBans(): Promise<SecurityBan[]> {
  const response = requireServerSuccess(
    await api.get<{ success: boolean; data: SecurityBan[] }>(
      '/api/network-security/bans'
    )
  )
  return response.data.data ?? []
}

async function deleteNetworkSecurityBan(ip?: string): Promise<number> {
  const response = requireServerSuccess(
    await api.delete<{ success: boolean; data: number }>(
      '/api/network-security/bans',
      {
        params: ip ? { ip } : undefined,
      }
    )
  )
  return response.data.data ?? 0
}

export function NetworkSecurityPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const bansQuery = useQuery({
    queryKey: NETWORK_SECURITY_BANS_QUERY_KEY,
    queryFn: getNetworkSecurityBans,
    refetchInterval: 5000,
  })
  const unbanMutation = useMutation({
    mutationFn: (ip?: string) => deleteNetworkSecurityBan(ip),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: NETWORK_SECURITY_BANS_QUERY_KEY,
      })
    },
  })

  const hasBans = Boolean(bansQuery.data?.length)

  let bansContent = (
    <p className='text-muted-foreground text-sm'>
      {t('No active network security bans')}
    </p>
  )
  if (bansQuery.isPending) {
    bansContent = (
      <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
    )
  } else if (bansQuery.data?.length) {
    bansContent = (
      <div className='overflow-x-auto rounded-lg border'>
        <table className='w-full min-w-[36rem] text-sm'>
          <thead className='bg-muted/50 text-left'>
            <tr>
              <th className='px-3 py-2 font-medium'>{t('IP address')}</th>
              <th className='px-3 py-2 font-medium'>{t('Reason')}</th>
              <th className='px-3 py-2 font-medium'>{t('Expires')}</th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Actions')}
              </th>
            </tr>
          </thead>
          <tbody>
            {bansQuery.data.map((ban) => (
              <tr key={`${ban.ip}-${ban.banned_at}`} className='border-t'>
                <td className='px-3 py-2 font-mono'>{ban.ip}</td>
                <td className='px-3 py-2'>{t(ban.reason)}</td>
                <td className='px-3 py-2 whitespace-nowrap'>
                  {formatDateTimeStr(new Date(ban.expires_at))}
                </td>
                <td className='px-3 py-2 text-right'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    disabled={unbanMutation.isPending}
                    onClick={() => unbanMutation.mutate(ban.ip)}
                  >
                    {t('Unban')}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader className='flex flex-row flex-wrap items-start justify-between gap-3'>
          <div className='space-y-1'>
            <CardTitle className='flex items-center gap-2'>
              <ShieldAlert className='size-4' />
              {t('Banned IP addresses')}
            </CardTitle>
            <CardDescription>
              {t('Active bans created by network path protection')}
            </CardDescription>
          </div>
          {hasBans ? (
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={unbanMutation.isPending}
              onClick={() => unbanMutation.mutate(undefined)}
            >
              {t('Clear all bans')}
            </Button>
          ) : null}
        </CardHeader>
        <CardContent>{bansContent}</CardContent>
      </Card>
      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Additional protection')}</CardTitle>
          <CardDescription>
            {t('More network security controls can be added here later')}
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  )
}
