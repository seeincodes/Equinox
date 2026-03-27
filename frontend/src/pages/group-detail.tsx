import { useParams, Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { useGroupHistory } from '../hooks/use-groups'
import { useAuth } from '../hooks/use-auth'
import { VenueComparison } from '../components/venue-comparison'
import { PriceChart } from '../components/price-chart'
import { RoutingPanel } from '../components/routing-panel'
import {
  confidenceColor,
  confidenceLevel,
  flagColor,
  flagLabel,
} from '../lib/utils'

export function GroupDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { user } = useAuth()
  const canRoute = user?.role === 'analyst' || user?.role === 'admin'

  const { data: groupsData, isLoading } = useQuery({
    queryKey: ['groups'],
    queryFn: () => api.getGroups(),
  })

  const group = groupsData?.groups?.find((g) => g.group_id === id)
  const { data: historyData } = useGroupHistory(id ?? '')

  if (isLoading) {
    return (
      <div className="py-12 text-center text-sm text-gray-500">Loading...</div>
    )
  }
  if (!group) {
    return (
      <div className="py-12 text-center">
        <p className="text-sm text-gray-500">Group not found</p>
        <Link to="/" className="mt-2 text-sm text-blue-600 hover:underline">
          Back to dashboard
        </Link>
      </div>
    )
  }

  const title = group.members?.[0]?.title ?? 'Unnamed Group'

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/"
          className="text-sm text-gray-500 hover:text-gray-700"
        >
          ← Back
        </Link>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-5">
        <div className="flex items-start justify-between">
          <div>
            <h1 className="text-lg font-semibold text-gray-900">{title}</h1>
            <p className="mt-1 text-xs text-gray-500">
              {group.match_method} match · Group {group.group_id.slice(0, 12)}
            </p>
          </div>
          <span
            className={`rounded-full px-2.5 py-1 text-xs font-medium ${confidenceColor(group.confidence_score)}`}
          >
            {confidenceLevel(group.confidence_score).toUpperCase()}{' '}
            {(group.confidence_score * 100).toFixed(1)}%
          </span>
        </div>
        {group.match_rationale && (
          <p className="mt-3 text-sm text-gray-600">
            {group.match_rationale}
          </p>
        )}
        {group.flags?.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-1.5">
            {group.flags.map((flag) => (
              <span
                key={flag}
                className={`rounded px-2 py-0.5 text-xs font-medium ${flagColor(flag)}`}
              >
                {flagLabel(flag)}
              </span>
            ))}
          </div>
        )}
      </div>

      {group.members && (
        <VenueComparison members={group.members} />
      )}

      <PriceChart history={historyData?.history ?? null} />

      {canRoute && <RoutingPanel groupId={group.group_id} />}
    </div>
  )
}
