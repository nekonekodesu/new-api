import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { NetworkSecurityPanel } from '../network-security-panel'

afterEach(() => vi.restoreAllMocks())

test('network security copy follows language changes in all seven locales', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const resources = { en, zh, 'zh-TW': zhTW, fr, ja, ru, vi: viLocale }
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: false,
    keySeparator: false,
    resources,
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <NetworkSecurityPanel />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByText('No active network security bans')
  for (const [locale, resource] of Object.entries(resources)) {
    await act(() => i18n.changeLanguage(locale))
    for (const key of [
      'Banned IP addresses',
      'Active bans created by network path protection',
      'No active network security bans',
      'Additional protection',
      'More network security controls can be added here later',
    ] as const) {
      const translated = resource.translation[key]
      expect(screen.getByText(translated)).toBeVisible()
      if (locale !== 'en') expect(translated).not.toBe(en.translation[key])
    }
  }
  client.clear()
})
