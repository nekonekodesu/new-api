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
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  convertRegexToWildcardPaths,
  mergeRegexConvertedRules,
  type RegexConversionErrorCode,
} from './regex-to-wildcard'

const NGINX_REGEX_EXAMPLE =
  '(?:\\.(?:env|git|svn|htaccess|htpasswd|bak|swp|sql|pem|key)|^/@fs/|^/__vite_rsc|wp-admin|wp-login|xmlrpc\\.php|phpinfo|actuator|_profiler)'

export type PathScanRegexConverterProps = {
  inputId: string
  label: string
  currentRulesText: string
  onUpdateRulesText: (nextRulesText: string) => void
}

export function PathScanRegexConverter(props: PathScanRegexConverterProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [regexValue, setRegexValue] = useState('')
  const [convertedCount, setConvertedCount] = useState(0)
  const [errorCode, setErrorCode] = useState<RegexConversionErrorCode | null>(
    null
  )
  const previousGeneratedRef = useRef<string[]>([])

  const handleRegexChange = (nextRegex: string) => {
    setRegexValue(nextRegex)

    if (!nextRegex.trim()) {
      const nextRules = mergeRegexConvertedRules(
        props.currentRulesText,
        previousGeneratedRef.current,
        []
      )
      previousGeneratedRef.current = []
      setConvertedCount(0)
      setErrorCode(null)
      if (nextRules !== props.currentRulesText) {
        props.onUpdateRulesText(nextRules)
      }
      return
    }

    const result = convertRegexToWildcardPaths(nextRegex)
    if (result.error !== null) {
      setErrorCode(result.error)
      return
    }

    setErrorCode(null)
    setConvertedCount(result.rules.length)
    const nextRules = mergeRegexConvertedRules(
      props.currentRulesText,
      previousGeneratedRef.current,
      result.rules
    )
    previousGeneratedRef.current = result.rules
    if (nextRules !== props.currentRulesText) {
      props.onUpdateRulesText(nextRules)
    }
  }

  let statusBadge: ReactNode = null
  if (errorCode === 'incomplete_regex') {
    statusBadge = (
      <Badge variant='warning'>{t('Incomplete regular expression')}</Badge>
    )
  } else if (errorCode === 'too_many_branches') {
    statusBadge = (
      <Badge variant='destructive'>
        {t('Regular expression expands to too many branches')}
      </Badge>
    )
  } else if (errorCode === 'invalid_regex') {
    statusBadge = (
      <Badge variant='destructive'>{t('Invalid regular expression')}</Badge>
    )
  } else if (convertedCount > 0) {
    statusBadge = (
      <Badge variant='secondary'>
        {t('Converted {{count}} wildcard rules', {
          count: formatNumber(convertedCount, locale),
        })}
      </Badge>
    )
  }

  return (
    <div className='border-border/80 bg-muted/20 min-w-0 space-y-1.5 rounded-lg border border-dashed p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <Label
          htmlFor={props.inputId}
          className='text-muted-foreground text-xs font-medium'
        >
          {props.label}
        </Label>
        {statusBadge}
      </div>
      <Input
        id={props.inputId}
        value={regexValue}
        autoComplete='off'
        spellCheck={false}
        placeholder={NGINX_REGEX_EXAMPLE}
        className='font-mono text-xs'
        onChange={(event) => handleRegexChange(event.target.value)}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Paste or type an Nginx/PCRE regular expression to convert it in real time into * wildcard rules in the filtering box below'
        )}
      </p>
    </div>
  )
}
