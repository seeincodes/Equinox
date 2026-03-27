import type { MatchFlag } from './types'

export function formatPrice(price: number): string {
  return (price * 100).toFixed(1) + '%'
}

export function formatLiquidity(liquidity: number): string {
  if (liquidity >= 1_000_000) return '$' + (liquidity / 1_000_000).toFixed(1) + 'M'
  if (liquidity >= 1_000) return '$' + (liquidity / 1_000).toFixed(0) + 'K'
  return '$' + liquidity.toFixed(0)
}

export function formatScore(score: number): string {
  return score.toFixed(4)
}

export function confidenceLevel(score: number): 'high' | 'medium' | 'low' {
  if (score >= 0.92) return 'high'
  if (score >= 0.78) return 'medium'
  return 'low'
}

export function confidenceColor(score: number): string {
  const level = confidenceLevel(score)
  if (level === 'high') return 'bg-green-100 text-green-800'
  if (level === 'medium') return 'bg-yellow-100 text-yellow-800'
  return 'bg-red-100 text-red-800'
}

export function flagColor(flag: MatchFlag): string {
  switch (flag) {
    case 'SETTLEMENT_DIVERGENCE':
      return 'bg-red-100 text-red-700'
    case 'STALE_PRICING_DATA':
      return 'bg-orange-100 text-orange-700'
    case 'LOW_CONFIDENCE':
      return 'bg-yellow-100 text-yellow-700'
    default:
      return 'bg-gray-100 text-gray-700'
  }
}

export function flagLabel(flag: MatchFlag): string {
  return flag.replace(/_/g, ' ')
}
