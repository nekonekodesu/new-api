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

export type RegexConversionErrorCode =
  | 'incomplete_regex'
  | 'invalid_regex'
  | 'too_many_branches'

export type RegexToWildcardResult = {
  rules: string[]
  error: RegexConversionErrorCode | null
}

type RegexAstNode =
  | { type: 'empty' }
  | { type: 'anchor-start' }
  | { type: 'anchor-end' }
  | { type: 'wildcard' }
  | { type: 'literal'; value: string }
  | { type: 'seq'; items: RegexAstNode[] }
  | { type: 'alt'; branches: RegexAstNode[] }

type ExpandedBranch = {
  anchoredStart: boolean
  anchoredEnd: boolean
  text: string
}

const MAX_EXPANDED_BRANCHES = 256

class RegexParseError extends Error {
  readonly code: RegexConversionErrorCode

  constructor(code: RegexConversionErrorCode) {
    super(code)
    this.code = code
  }
}

function collapseStars(input: string): string {
  return input.replaceAll(/\*+/g, '*')
}

function unwrapRegexInputLine(rawLine: string): string {
  let line = rawLine.trim()
  if (!line) {
    return ''
  }

  // Strip optional Nginx `location ~* <pattern> {` wrapper
  const nginxLocationMatch = /^location\s+~\*?\s+(.+?)\s*\{?\s*$/i.exec(line)
  if (nginxLocationMatch) {
    line = nginxLocationMatch[1].trim()
  }

  // Strip optional surrounding quotes
  if (
    (line.startsWith('"') && line.endsWith('"') && line.length >= 2) ||
    (line.startsWith("'") && line.endsWith("'") && line.length >= 2)
  ) {
    line = line.slice(1, -1).trim()
  }

  // Strip Nginx `~*` / `~` or `regex:` prefix
  if (line.startsWith('~*')) {
    line = line.slice(2).trim()
  } else if (line.startsWith('~')) {
    line = line.slice(1).trim()
  } else if (line.toLowerCase().startsWith('regex:')) {
    line = line.slice(6).trim()
  }

  // Strip JS regex literal `/pattern/flags` when flags are present or pattern is anchored/escaped
  const jsSlashMatch = /^\/(.+)\/([igmsuy]*)$/.exec(line)
  if (jsSlashMatch) {
    const inner = jsSlashMatch[1]
    const flags = jsSlashMatch[2]
    if (
      flags.length > 0 ||
      inner.startsWith('^') ||
      inner.startsWith('\\') ||
      inner.endsWith('$')
    ) {
      line = inner
    }
  }

  return line
}

function parseRegexLineToAst(pattern: string): RegexAstNode {
  let pos = 0

  function parseAlternation(): RegexAstNode {
    const branches: RegexAstNode[] = [parseSequence()]
    while (pos < pattern.length && pattern[pos] === '|') {
      pos += 1
      branches.push(parseSequence())
    }
    if (branches.length === 1) {
      return branches[0]
    }
    return { type: 'alt', branches }
  }

  function parseSequence(): RegexAstNode {
    const items: RegexAstNode[] = []
    while (pos < pattern.length) {
      const ch = pattern[pos]
      if (ch === '|' || ch === ')') {
        break
      }
      const atom = parseAtom()
      const quantified = parseQuantifier(atom)
      if (quantified.type !== 'empty') {
        items.push(quantified)
      }
    }
    if (items.length === 0) {
      return { type: 'empty' }
    }
    if (items.length === 1) {
      return items[0]
    }
    return { type: 'seq', items }
  }

  function parseCharacterClass(): RegexAstNode {
    // Advance past '['
    pos += 1
    if (pos >= pattern.length) {
      throw new RegexParseError('incomplete_regex')
    }
    const start = pos
    if (pattern[pos] === '^') {
      pos += 1
    }
    if (pos < pattern.length && pattern[pos] === ']') {
      pos += 1
    }
    while (pos < pattern.length) {
      if (pattern[pos] === '\\') {
        if (pos + 1 >= pattern.length) {
          throw new RegexParseError('incomplete_regex')
        }
        pos += 2
        continue
      }
      if (pattern[pos] === ']') {
        const content = pattern.slice(start, pos)
        pos += 1
        if (content.length === 1 && content !== '^') {
          return { type: 'literal', value: content }
        }
        if (
          content.length === 2 &&
          !content.startsWith('^') &&
          !content.startsWith('\\') &&
          content[0].toLowerCase() === content[1].toLowerCase()
        ) {
          return { type: 'literal', value: content[0].toLowerCase() }
        }
        return { type: 'wildcard' }
      }
      pos += 1
    }
    throw new RegexParseError('incomplete_regex')
  }

  function parseGroup(): RegexAstNode {
    // Advance past '('
    pos += 1
    if (pos >= pattern.length) {
      throw new RegexParseError('incomplete_regex')
    }

    if (pattern[pos] === '?') {
      pos += 1
      if (pos >= pattern.length) {
        throw new RegexParseError('incomplete_regex')
      }

      // Non-capturing group `(?:...)`
      if (pattern[pos] === ':') {
        pos += 1
        const inner = parseAlternation()
        expectClosingParen()
        return inner
      }

      // Lookahead `(?=...)` or `(?!...)`
      if (pattern[pos] === '=' || pattern[pos] === '!') {
        const isPositive = pattern[pos] === '='
        pos += 1
        const inner = parseAlternation()
        expectClosingParen()
        return isPositive ? inner : { type: 'empty' }
      }

      // Named capture `(?<name>...)` or `(?P<name>...)`
      if (
        pattern[pos] === '<' ||
        (pattern[pos] === 'P' && pattern[pos + 1] === '<')
      ) {
        if (pattern[pos] === 'P') {
          pos += 1
        }
        const closeAngle = pattern.indexOf('>', pos + 1)
        if (closeAngle === -1) {
          throw new RegexParseError('incomplete_regex')
        }
        pos = closeAngle + 1
        const inner = parseAlternation()
        expectClosingParen()
        return inner
      }

      // Inline flags `(?i)` or scoped flags `(?i:...)`
      const flagMatch = /^[a-zA-Z-]+/.exec(pattern.slice(pos))
      if (flagMatch) {
        pos += flagMatch[0].length
        if (pos >= pattern.length) {
          throw new RegexParseError('incomplete_regex')
        }
        if (pattern[pos] === ')') {
          pos += 1
          return { type: 'empty' }
        }
        if (pattern[pos] === ':') {
          pos += 1
          const inner = parseAlternation()
          expectClosingParen()
          return inner
        }
      }

      throw new RegexParseError('invalid_regex')
    }

    const inner = parseAlternation()
    expectClosingParen()
    return inner
  }

  function expectClosingParen(): void {
    if (pos >= pattern.length) {
      throw new RegexParseError('incomplete_regex')
    }
    if (pattern[pos] !== ')') {
      throw new RegexParseError('invalid_regex')
    }
    pos += 1
  }

  function parseAtom(): RegexAstNode {
    const ch = pattern[pos]
    if (ch === '^') {
      pos += 1
      return { type: 'anchor-start' }
    }
    if (ch === '$') {
      pos += 1
      return { type: 'anchor-end' }
    }
    if (ch === '.') {
      pos += 1
      return { type: 'wildcard' }
    }
    if (ch === '(') {
      return parseGroup()
    }
    if (ch === '[') {
      return parseCharacterClass()
    }
    if (ch === '\\') {
      if (pos + 1 >= pattern.length) {
        throw new RegexParseError('incomplete_regex')
      }
      const next = pattern[pos + 1]
      pos += 2
      if (
        next === 'd' ||
        next === 'D' ||
        next === 'w' ||
        next === 'W' ||
        next === 's' ||
        next === 'S'
      ) {
        return { type: 'wildcard' }
      }
      if (next === 'b' || next === 'B') {
        return { type: 'empty' }
      }
      return { type: 'literal', value: next }
    }
    if (ch === '*') {
      // Allow glob-like leading '*' inside converter input gracefully
      pos += 1
      return { type: 'wildcard' }
    }
    if (ch === '+' || ch === '?') {
      throw new RegexParseError('invalid_regex')
    }
    pos += 1
    return { type: 'literal', value: ch }
  }

  function parseQuantifier(atom: RegexAstNode): RegexAstNode {
    if (pos >= pattern.length) {
      return atom
    }
    const ch = pattern[pos]
    if (ch === '*') {
      pos += 1
      if (pos < pattern.length && pattern[pos] === '?') {
        pos += 1
      }
      return { type: 'wildcard' }
    }
    if (ch === '+') {
      pos += 1
      if (pos < pattern.length && pattern[pos] === '?') {
        pos += 1
      }
      if (atom.type === 'wildcard' || atom.type === 'empty') {
        return atom
      }
      return { type: 'seq', items: [atom, { type: 'wildcard' }] }
    }
    if (ch === '?') {
      pos += 1
      if (pos < pattern.length && pattern[pos] === '?') {
        pos += 1
      }
      if (atom.type === 'empty' || atom.type === 'wildcard') {
        return { type: 'wildcard' }
      }
      return { type: 'alt', branches: [{ type: 'empty' }, atom] }
    }
    if (ch === '{') {
      const rest = pattern.slice(pos)
      if (/^\{\d+(?:,\d*)?$/.test(rest)) {
        throw new RegexParseError('incomplete_regex')
      }
      const rangeMatch = /^\{(\d+)(?:,(\d*))?\}(\?)?/.exec(rest)
      if (rangeMatch) {
        pos += rangeMatch[0].length
        const minCount = Number(rangeMatch[1])
        const hasComma = rangeMatch[2] !== undefined
        let maxCount = minCount
        if (hasComma) {
          maxCount =
            rangeMatch[2] === ''
              ? Number.POSITIVE_INFINITY
              : Number(rangeMatch[2])
        }
        if (minCount === 0) {
          return { type: 'wildcard' }
        }
        const repeatTimes = Math.min(minCount, 4)
        const repeated: RegexAstNode[] = Array.from(
          { length: repeatTimes },
          () => atom
        )
        if (maxCount > repeatTimes) {
          repeated.push({ type: 'wildcard' })
        }
        return { type: 'seq', items: repeated }
      }
    }
    return atom
  }

  const root = parseAlternation()
  if (pos < pattern.length) {
    throw new RegexParseError('invalid_regex')
  }
  return root
}

function concatBranches(
  left: ExpandedBranch,
  right: ExpandedBranch
): ExpandedBranch {
  const anchoredStart =
    left.anchoredStart ||
    (left.text === '' && !left.anchoredEnd && right.anchoredStart)
  const anchoredEnd =
    right.anchoredEnd ||
    (right.text === '' && !right.anchoredStart && left.anchoredEnd)
  return {
    anchoredStart,
    anchoredEnd,
    text: collapseStars(left.text + right.text),
  }
}

function expandAst(node: RegexAstNode): ExpandedBranch[] {
  switch (node.type) {
    case 'empty':
      return [{ anchoredStart: false, anchoredEnd: false, text: '' }]
    case 'anchor-start':
      return [{ anchoredStart: true, anchoredEnd: false, text: '' }]
    case 'anchor-end':
      return [{ anchoredStart: false, anchoredEnd: true, text: '' }]
    case 'wildcard':
      return [{ anchoredStart: false, anchoredEnd: false, text: '*' }]
    case 'literal':
      return [{ anchoredStart: false, anchoredEnd: false, text: node.value }]
    case 'alt': {
      const results: ExpandedBranch[] = []
      for (const branch of node.branches) {
        const sub = expandAst(branch)
        if (results.length + sub.length > MAX_EXPANDED_BRANCHES) {
          throw new RegexParseError('too_many_branches')
        }
        results.push(...sub)
      }
      return results
    }
    case 'seq': {
      let acc: ExpandedBranch[] = [
        { anchoredStart: false, anchoredEnd: false, text: '' },
      ]
      for (const item of node.items) {
        const nextList = expandAst(item)
        if (acc.length * nextList.length > MAX_EXPANDED_BRANCHES) {
          throw new RegexParseError('too_many_branches')
        }
        const combined: ExpandedBranch[] = []
        for (const left of acc) {
          for (const right of nextList) {
            combined.push(concatBranches(left, right))
          }
        }
        acc = combined
      }
      return acc
    }
  }
}

function branchToWildcardPaths(branch: ExpandedBranch): string[] {
  let text = branch.text
  let anchoredEnd = branch.anchoredEnd

  // URL path matching operates on `URL.Path` (before `?` query or `#` fragment)
  const queryOrHashIdx = text.search(/[?#]/)
  if (queryOrHashIdx !== -1) {
    text = text.slice(0, queryOrHashIdx)
    anchoredEnd = true
  }

  text = collapseStars(text.trim())
  if (!text) {
    if (branch.anchoredStart && anchoredEnd) {
      return ['/']
    }
    return []
  }
  if (text === '*') {
    return ['/*']
  }

  const suffix = anchoredEnd || text.endsWith('*') ? '' : '*'

  if (branch.anchoredStart) {
    const withSlash = text.startsWith('/') ? text : `/${text}`
    return [collapseStars(`${withSlash}${suffix}`)]
  }

  if (text === '/') {
    return ['/*']
  }
  if (text.startsWith('/*')) {
    return [collapseStars(`${text}${suffix}`)]
  }
  if (text.startsWith('/')) {
    const rest = text.slice(1)
    return [
      collapseStars(`/${rest}${suffix}`),
      collapseStars(`/*/${rest}${suffix}`),
    ]
  }
  return [collapseStars(`/*${text}${suffix}`)]
}

/**
 * Converts one or more Nginx / PCRE / JS regular expressions into deduplicated
 * non-regex wildcard (`*`) URL path rules in real time.
 */
export function convertRegexToWildcardPaths(
  rawInput: string
): RegexToWildcardResult {
  const lines = rawInput.split('\n')
  const seen = new Set<string>()
  const rules: string[] = []

  try {
    for (const rawLine of lines) {
      const cleaned = unwrapRegexInputLine(rawLine)
      if (!cleaned) {
        continue
      }
      const ast = parseRegexLineToAst(cleaned)
      const branches = expandAst(ast)
      for (const branch of branches) {
        for (const wildcardPath of branchToWildcardPaths(branch)) {
          if (!seen.has(wildcardPath)) {
            seen.add(wildcardPath)
            rules.push(wildcardPath)
          }
        }
      }
    }
    return { rules, error: null }
  } catch (err) {
    if (err instanceof RegexParseError) {
      return { rules: [], error: err.code }
    }
    return { rules: [], error: 'invalid_regex' }
  }
}

/**
 * Replaces previously auto-generated wildcard lines in `currentText` with
 * `nextGenerated` while preserving any manual rules added directly in the main
 * filtering rules textarea.
 */
export function mergeRegexConvertedRules(
  currentText: string,
  previousGenerated: readonly string[],
  nextGenerated: readonly string[]
): string {
  const previousSet = new Set(previousGenerated)
  const nextSet = new Set(nextGenerated)
  const manualLines: string[] = []
  const seen = new Set<string>()

  for (const rawLine of currentText.split('\n')) {
    const trimmed = rawLine.trim()
    if (!trimmed) {
      continue
    }
    if (previousSet.has(trimmed) || nextSet.has(trimmed)) {
      continue
    }
    if (!seen.has(trimmed)) {
      seen.add(trimmed)
      manualLines.push(trimmed)
    }
  }

  const merged = [...manualLines]
  for (const rule of nextGenerated) {
    if (!seen.has(rule)) {
      seen.add(rule)
      merged.push(rule)
    }
  }
  return merged.join('\n')
}
