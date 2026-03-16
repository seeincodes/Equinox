import type { Market } from '../lib/types'
import { formatPrice, formatLiquidity } from '../lib/utils'

interface Props {
  members: Market[]
}

const dimensions = [
  { key: 'yes_price', label: 'YES Price', format: formatPrice, higher: true },
  { key: 'no_price', label: 'NO Price', format: formatPrice, higher: false },
  { key: 'spread', label: 'Spread', format: formatPrice, higher: false },
  { key: 'liquidity', label: 'Liquidity', format: formatLiquidity, higher: true },
] as const

export function VenueComparison({ members }: Props) {
  if (members.length < 2) return null

  return (
    <div className="rounded-lg border border-gray-200 bg-white">
      <div className="grid grid-cols-3 border-b border-gray-200 px-4 py-2">
        <div className="text-xs font-medium text-gray-500">Dimension</div>
        {members.slice(0, 2).map((m) => (
          <div key={m.id} className="text-center text-xs font-medium text-gray-500">
            {m.venue}
          </div>
        ))}
      </div>
      {dimensions.map((dim) => {
        const vals = members.slice(0, 2).map((m) => m[dim.key] as number)
        const winIdx = dim.higher
          ? vals[0] >= vals[1] ? 0 : 1
          : vals[0] <= vals[1] ? 0 : 1

        return (
          <div key={dim.key} className="grid grid-cols-3 border-b border-gray-100 px-4 py-2.5 last:border-0">
            <div className="text-sm text-gray-700">{dim.label}</div>
            {vals.map((v, i) => (
              <div
                key={i}
                className={`text-center text-sm font-medium ${
                  i === winIdx ? 'text-green-700' : 'text-gray-600'
                }`}
              >
                {dim.format(v)}
                {i === winIdx && <span className="ml-1 text-xs text-green-500">●</span>}
              </div>
            ))}
          </div>
        )
      })}
      <div className="grid grid-cols-3 border-t border-gray-200 px-4 py-2.5">
        <div className="text-sm text-gray-700">Status</div>
        {members.slice(0, 2).map((m) => (
          <div key={m.id} className="text-center text-sm">
            <span
              className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                m.status === 'OPEN'
                  ? 'bg-green-100 text-green-800'
                  : 'bg-gray-100 text-gray-600'
              }`}
            >
              {m.status}
            </span>
          </div>
        ))}
      </div>
      <div className="grid grid-cols-3 border-t border-gray-100 px-4 py-2.5">
        <div className="text-sm text-gray-700">Settlement</div>
        {members.slice(0, 2).map((m) => (
          <div key={m.id} className="text-center text-xs text-gray-500">
            {m.settlement_mechanism.replace(/_/g, ' ')}
          </div>
        ))}
      </div>
    </div>
  )
}
