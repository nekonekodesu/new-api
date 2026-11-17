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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  createNetworkBehaviorSchema,
  type NetworkBehaviorFormValues,
} from './network-behavior-schema'
import { PathScanRegexConverter } from './path-scan-regex-converter'

type NetworkBehaviorSectionProps = {
  defaultValues: {
    'network_setting.primary_dns': string
    'network_setting.fallback_dns': string
    'path_scan_setting.path_scan_enabled': boolean
    'path_scan_setting.path_scan_window_seconds': number
    'path_scan_setting.path_scan_max_requests': number
    'path_scan_setting.path_scan_ban_duration': number
    'path_scan_setting.path_scan_ban_duration_unit':
      | 'second'
      | 'minute'
      | 'hour'
      | 'day'
      | 'year'
    'path_scan_setting.path_scan_direct_paths': string
    'path_scan_setting.path_scan_counted_paths': string
  }
}

export function NetworkBehaviorSection(props: NetworkBehaviorSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(() => createNetworkBehaviorSchema(t), [t])

  const formDefaults = useMemo<NetworkBehaviorFormValues>(
    () => ({
      network_setting: {
        primary_dns: props.defaultValues['network_setting.primary_dns'],
        fallback_dns: props.defaultValues['network_setting.fallback_dns'],
      },
      path_scan_setting: {
        path_scan_enabled:
          props.defaultValues['path_scan_setting.path_scan_enabled'],
        path_scan_window_seconds:
          props.defaultValues['path_scan_setting.path_scan_window_seconds'],
        path_scan_max_requests:
          props.defaultValues['path_scan_setting.path_scan_max_requests'],
        path_scan_ban_duration:
          props.defaultValues['path_scan_setting.path_scan_ban_duration'],
        path_scan_ban_duration_unit:
          props.defaultValues['path_scan_setting.path_scan_ban_duration_unit'],
        path_scan_direct_paths:
          props.defaultValues['path_scan_setting.path_scan_direct_paths'],
        path_scan_counted_paths:
          props.defaultValues['path_scan_setting.path_scan_counted_paths'],
      },
    }),
    [props.defaultValues]
  )

  const form = useForm<NetworkBehaviorFormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const onSubmit = async (values: NetworkBehaviorFormValues) => {
    const nextPrimary = values.network_setting.primary_dns.trim()
    const nextFallback = values.network_setting.fallback_dns.trim()
    const updates: Array<{ key: string; value: string | boolean | number }> = []
    const valuesByKey: Record<string, string | boolean | number> = {
      'network_setting.primary_dns': nextPrimary,
      'network_setting.fallback_dns': nextFallback,
      ...Object.fromEntries(
        Object.entries(values.path_scan_setting).map(([key, value]) => [
          `path_scan_setting.${key}`,
          value,
        ])
      ),
    }
    for (const [key, value] of Object.entries(valuesByKey)) {
      if (
        String(value) !==
        String(props.defaultValues[key as keyof typeof props.defaultValues])
      ) {
        updates.push({ key, value })
      }
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Network Behavior')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Configure primary and fallback DNS servers used first when resolving outbound requests, preventing local DNS loops such as Wings built-in DNS.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='network_setting.primary_dns'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Default DNS')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='1.1.1.1'
                    autoComplete='off'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Primary DNS server prioritized for outbound domain resolution (IPv4/IPv6 with optional :port, defaults to port 53). Leave empty to use system DNS.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='network_setting.fallback_dns'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Fallback DNS')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='8.8.8.8'
                    autoComplete='off'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Secondary DNS server queried when the default DNS fails or times out, before falling back to the system resolver.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid min-w-0 gap-4 sm:grid-cols-2 lg:col-span-2 lg:grid-cols-5'>
            <FormField
              control={form.control}
              name='path_scan_setting.path_scan_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable path scan protection')}</FormLabel>
                    <FormDescription>
                      {t('Block suspicious directory and file path scanning')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='path_scan_setting.path_scan_window_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Scan window (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={86400}
                      value={field.value}
                      onChange={(event) =>
                        field.onChange(Number(event.target.value))
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Time window used to count repeated scan requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='path_scan_setting.path_scan_max_requests'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Scan requests in window')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={100000}
                      value={field.value}
                      onChange={(event) =>
                        field.onChange(Number(event.target.value))
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Number of counted paths before the IP is banned')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='path_scan_setting.path_scan_ban_duration'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Ban duration')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={36500}
                      value={field.value}
                      onChange={(event) =>
                        field.onChange(Number(event.target.value))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='path_scan_setting.path_scan_ban_duration_unit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Ban duration unit')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='second'>{t('Seconds')}</SelectItem>
                      <SelectItem value='minute'>{t('Minutes')}</SelectItem>
                      <SelectItem value='hour'>{t('Hours')}</SelectItem>
                      <SelectItem value='day'>{t('Days')}</SelectItem>
                      <SelectItem value='year'>{t('Years')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='path_scan_setting.path_scan_direct_paths'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Directly block paths')}</FormLabel>
                <PathScanRegexConverter
                  inputId='path-scan-direct-regex-converter'
                  label={t('Regex converter (Direct block)')}
                  currentRulesText={field.value}
                  onUpdateRulesText={field.onChange}
                />
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder='/admin&#10;/*wp-admin*&#10;/*.env*'
                    className='font-mono text-xs'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Primary filtering rules (one per line, supports * wildcards and regex); matching paths immediately ban the IP and return HTTP 403'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='path_scan_setting.path_scan_counted_paths'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Counted scan paths')}</FormLabel>
                <PathScanRegexConverter
                  inputId='path-scan-counted-regex-converter'
                  label={t('Regex converter (Counted scan)')}
                  currentRulesText={field.value}
                  onUpdateRulesText={field.onChange}
                />
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder='/wp-login.php&#10;/*.git*&#10;/*xmlrpc.php*'
                    className='font-mono text-xs'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Primary filtering rules (one per line, supports * wildcards and regex); repeated matches within the window trigger a ban'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
