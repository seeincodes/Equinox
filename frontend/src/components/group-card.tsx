import { Link } from 'react-router'
import type { EquivalenceGroup } from '../lib/types'
import {
  formatPrice,
  formatLiquidity,
  confidenceColor,
  confidenceLevel,
  flagColor,
  flagLabel,
} from '../lib/utils'
import { useAuth } from '../hooks/use-auth'

export function GroupCard({ group }: { group: EquivalenceGroup }) {
  const { user } = useAuth()
  const title = group.members?.[0]?.title ?? 'Unnamed Group'
  const canRoute = user?.role === 'analyst' || user?.role === 'admin'

  return (
    <Link
      to={`/groups/${group.group_id}`}
      className="block rounded-lg border border-gray-200 bg-white p-5 shadow-sm transition-shadow hover:shadow-md"
    >
      <div className="mb-3 flex items-start justify-between">
        <h3 className="text-sm font-semibold text-gray-900 line-clamp-2">
          {title}
        </h3>
        <span
          className={`ml-2 shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${confidenceColor(group.confidence_score)}`}
        >
          {confidenceLevel(group.confidence_score).toUpperCase()}{' '}
          {(group.confidence_score * 100).toFixed(0)}%
        </span>
      </div>

      {group.members && group.members.length > 0 && (
        <div className="mb-3 grid grid-cols-2 gap-3">
          {group.members.map((m) => (
            <div
              key={m.id}
              className="rounded-md border border-gray-100 bg-gray-50 p-2.5"
            >
              <div className="mb-1 text-xs font-medium text-gray-500">
                {m.venue}
              </div>
              <div className="flex items-baseline justify-between">
                <span className="text-lg font-semibold text-gray-900">
                  {formatPrice(m.yes_price)}
                </span>
                <span className="text-xs text-gray-400">YES</span>
              </div>
              <div className="mt-1 flex justify-between text-xs text-gray-500">
                <span>{formatLiquidity(m.liquidity)}</span>
                <span>spread {formatPrice(m.spread)}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-1.5">
        {group.flags?.map((flag) => (
          <span
            key={flag}
            className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${flagColor(flag)}`}
          >
            {flagLabel(flag)}
          </span>
        ))}
        {canRoute && (
          <span className="ml-auto rounded-md bg-blue-600 px-2.5 py-1 text-xs font-medium text-white">
            Route →
          </span>
        )}
      </div>
    </Link>
  )
}
