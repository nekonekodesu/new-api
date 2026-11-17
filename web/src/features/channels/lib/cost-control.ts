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
import type {
  ChannelBrief,
  CostControlUpstreamAlloc,
  CostControlWindow,
  GroupCostControlPolicy,
  GroupRealtimeProfitSnapshot,
  UpstreamSourceBrief,
} from '../api'

export const DIRECT_UPSTREAM_ID = 'direct'

export function normalizeCostControlPolicy(
  raw?: GroupCostControlPolicy
): GroupCostControlPolicy {
  const windowVal: CostControlWindow =
    raw?.window === 'today' ||
    raw?.window === '24h' ||
    raw?.window === '7d' ||
    raw?.window === '30d'
      ? raw.window
      : '24h'
  const maxRatio =
    typeof raw?.max_cost_ratio_pct === 'number' &&
    Number.isFinite(raw.max_cost_ratio_pct) &&
    raw.max_cost_ratio_pct > 0
      ? raw.max_cost_ratio_pct
      : 100
  const minProfit =
    typeof raw?.min_net_profit_usd === 'number' &&
    Number.isFinite(raw.min_net_profit_usd)
      ? raw.min_net_profit_usd
      : 0
  const sensitivity =
    typeof raw?.overflow_sensitivity === 'number' &&
    Number.isFinite(raw.overflow_sensitivity) &&
    raw.overflow_sensitivity > 0
      ? raw.overflow_sensitivity
      : 1

  return {
    window: windowVal,
    max_cost_ratio_pct: maxRatio,
    min_net_profit_usd: minProfit,
    overflow_sensitivity: sensitivity,
    upstream_allocations: { ...raw?.upstream_allocations },
    channel_rules: { ...raw?.channel_rules },
  }
}

export function resolveUpstreamAlloc(
  upstreamId: string,
  policy: GroupCostControlPolicy
): CostControlUpstreamAlloc {
  const existing = policy.upstream_allocations[upstreamId]
  if (existing) {
    return {
      enabled: Boolean(existing.enabled),
      weight: Math.max(0, Math.round(existing.weight ?? 0)),
    }
  }
  return { enabled: true, weight: 100 }
}

export function resolveEffectiveChannelUpstream(
  ch: ChannelBrief,
  policy: GroupCostControlPolicy,
  upstreamSources: UpstreamSourceBrief[]
): { id: string; name: string; isDirect: boolean; isOverridden: boolean } {
  const rule = policy.channel_rules[String(ch.id)]
  const overrideId = rule?.upstream_id?.trim()
  if (overrideId) {
    if (overrideId.toLowerCase() === DIRECT_UPSTREAM_ID) {
      return {
        id: DIRECT_UPSTREAM_ID,
        name: 'Direct',
        isDirect: true,
        isOverridden: true,
      }
    }
    const matched = upstreamSources.find((src) => src.id === overrideId)
    if (matched) {
      return {
        id: matched.id,
        name: matched.name,
        isDirect: matched.is_direct,
        isOverridden: true,
      }
    }
  }

  const autoId = ch.upstream_source_id?.trim() || DIRECT_UPSTREAM_ID
  if (autoId === DIRECT_UPSTREAM_ID) {
    return {
      id: DIRECT_UPSTREAM_ID,
      name: 'Direct',
      isDirect: true,
      isOverridden: false,
    }
  }
  const matched = upstreamSources.find((src) => src.id === autoId)
  return {
    id: autoId,
    name: matched?.name || ch.upstream_source_name || autoId,
    isDirect: Boolean(matched?.is_direct),
    isOverridden: false,
  }
}

export function resolveEffectiveChannelPriority(
  ch: ChannelBrief,
  policy: GroupCostControlPolicy
): number {
  const rule = policy.channel_rules[String(ch.id)]
  if (typeof rule?.priority === 'number' && Number.isFinite(rule.priority)) {
    return Math.round(rule.priority)
  }
  return ch.priority ?? 0
}

export function resolveEffectiveChannelWeight(
  ch: ChannelBrief,
  policy: GroupCostControlPolicy
): number {
  const rule = policy.channel_rules[String(ch.id)]
  if (typeof rule?.weight === 'number' && Number.isFinite(rule.weight)) {
    return Math.max(0, Math.round(rule.weight))
  }
  return ch.weight > 0 ? ch.weight : 100
}

export function evaluateRealtimeProfitWithPolicy(
  snapshot: GroupRealtimeProfitSnapshot | undefined,
  policy: GroupCostControlPolicy
) {
  const totalCalls = snapshot?.total_calls ?? 0
  const revenueUSD = snapshot?.revenue_usd ?? 0
  const costUSD = snapshot?.cost_usd ?? 0
  const netProfitUSD = revenueUSD - costUSD

  let netMarginPct = 0
  let costRatioPct = 0
  if (revenueUSD > 1e-9) {
    netMarginPct = ((revenueUSD - costUSD) / revenueUSD) * 100
    costRatioPct = (costUSD / revenueUSD) * 100
  } else if (costUSD > 1e-9) {
    netMarginPct = -100
    costRatioPct = 100
  }

  const ratioBound = revenueUSD * (policy.max_cost_ratio_pct / 100)
  let allowedCostUSD = ratioBound
  if (policy.min_net_profit_usd < 0) {
    allowedCostUSD = ratioBound - policy.min_net_profit_usd
  } else if (policy.min_net_profit_usd > 0) {
    allowedCostUSD = Math.min(
      ratioBound,
      Math.max(0, revenueUSD - policy.min_net_profit_usd)
    )
  }

  const profitCushionUSD = allowedCostUSD - costUSD
  const isOverCost = costUSD > 1e-9 && costUSD > allowedCostUSD + 1e-9
  let divertProbPct = 0
  if (isOverCost) {
    const deficit = costUSD - allowedCostUSD
    const scale = Math.max(allowedCostUSD, revenueUSD, 0.05)
    const sens =
      policy.overflow_sensitivity > 0 ? policy.overflow_sensitivity : 1
    const rawSeverity = (deficit / scale) * sens
    divertProbPct =
      Math.min(1, 0.35 + 0.65 * (rawSeverity / (0.35 + rawSeverity))) * 100
  }

  return {
    totalCalls,
    revenueUSD,
    costUSD,
    netProfitUSD,
    netMarginPct,
    costRatioPct,
    allowedCostUSD,
    profitCushionUSD,
    isOverCost,
    divertProbPct,
  }
}
