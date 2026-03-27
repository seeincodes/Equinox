import type { VenueScore, Venue } from '../lib/types'
import { formatScore } from '../lib/utils'

interface Props {
  breakdown: Record<string, VenueScore>
  selectedVenue: Venue
}

const scoreDimensions = [
  { key: 'price_quality', label: 'Price Quality', weight: '40%' },
  { key: 'liquidity', label: 'Liquidity', weight: '35%' },
  { key: 'spread_quality', label: 'Spread Quality', weight: '15%' },
  { key: 'market_status', label: 'Market Status', weight: '10%' },
] as const

export function ScoringBreakdown({ breakdown, selectedVenue }: Props) {
  const venues = Object.keys(breakdown)

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
      <table className="w-full min-w-[400px] text-sm">
        <thead className="bg-gray-50">
          <tr>
            <th className="px-3 py-2 text-left text-xs font-medium text-gray-500">
              Dimension
            </th>
            <th className="px-3 py-2 text-right text-xs font-medium text-gray-500">
              Weight
            </th>
            {venues.map((v) => (
              <th
                key={v}
                className={`px-3 py-2 text-right text-xs font-medium ${
                  v === selectedVenue ? 'text-green-700' : 'text-gray-500'
                }`}
              >
                {v}
                {v === selectedVenue && ' ✓'}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {scoreDimensions.map((dim) => (
            <tr key={dim.key} className="border-t border-gray-100">
              <td className="px-3 py-2 text-gray-700">{dim.label}</td>
              <td className="px-3 py-2 text-right text-gray-400">
                {dim.weight}
              </td>
              {venues.map((v) => {
                const score = breakdown[v]?.[dim.key] ?? 0
                return (
                  <td
                    key={v}
                    className={`px-3 py-2 text-right font-mono ${
                      v === selectedVenue
                        ? 'font-medium text-green-700'
                        : 'text-gray-600'
                    }`}
                  >
                    {formatScore(score)}
                  </td>
                )
              })}
            </tr>
          ))}
          <tr className="border-t-2 border-gray-200 bg-gray-50 font-medium">
            <td className="px-3 py-2 text-gray-900">Total</td>
            <td className="px-3 py-2 text-right text-gray-400">100%</td>
            {venues.map((v) => (
              <td
                key={v}
                className={`px-3 py-2 text-right font-mono ${
                  v === selectedVenue
                    ? 'font-bold text-green-700'
                    : 'text-gray-700'
                }`}
              >
                {formatScore(breakdown[v]?.total ?? 0)}
              </td>
            ))}
          </tr>
        </tbody>
      </table>
    </div>
  )
}
