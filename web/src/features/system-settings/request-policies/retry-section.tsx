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
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { SettingsCard } from '../components/settings-card'
import {
  SettingsControlChildren,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { safeNumberFieldProps } from '../utils/numeric-field'
import type { RoutingPolicyFormValues } from './routing-form'

export function RetrySection() {
  const { t } = useTranslation()
  const form = useFormContext<RoutingPolicyFormValues>()
  const cooldownEnabled = useWatch({
    control: form.control,
    name: 'RetryCooldownEnabled',
  })
  return (
    <SettingsCard title={t('Retry budget')} className='shadow-none'>
      <div className='space-y-4'>
        <FormField
          control={form.control}
          name='RetryTimes'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Maximum retries')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0}
                  max={99}
                  step={1}
                  {...safeNumberFieldProps(field)}
                />
              </FormControl>
              <FormDescription>
                {t('Excludes the first attempt. Counted per group.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='AutomaticRetryStatusCodes'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Auto-retry status codes')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  placeholder={t('e.g. 401, 403, 429, 500-599')}
                />
              </FormControl>
              <FormDescription>
                {t('2xx, 504 and 524 are always excluded.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='RetryCooldownEnabled'
          render={({ field }) => (
            <SettingsSwitchField
              controlId='RetryCooldownEnabled'
              checked={field.value}
              onCheckedChange={field.onChange}
              label={t('Channel failure cooldown')}
              description={t(
                'Skips a channel and model after repeated failures in the window. If all are cooling down, one more round ignores the cooldown.'
              )}
            />
          )}
        />
        <SettingsControlChildren
          role='group'
          aria-label={t('Channel failure cooldown options')}
        >
          <div className='grid gap-4 sm:grid-cols-3'>
            {(
              [
                {
                  name: 'RetryCooldownWindowSeconds',
                  label: t('Failure window (seconds)'),
                },
                {
                  name: 'RetryCooldownFailureThreshold',
                  label: t('Failures before cooldown'),
                },
                {
                  name: 'RetryCooldownSeconds',
                  label: t('Cooldown duration (seconds)'),
                },
              ] as const
            ).map((item) => (
              <FormField
                key={item.name}
                control={form.control}
                name={item.name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{item.label}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={!cooldownEnabled}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>
        </SettingsControlChildren>
      </div>
    </SettingsCard>
  )
}
