import { useState, useMemo } from 'react'
import { useGroups } from '../hooks/use-groups'
import { GroupCard } from '../components/group-card'
import type { Venue, MarketStatus } from '../lib/types'

export function DashboardPage() {
  const [minConfidence, setMinConfidence] = useState(0)
  const [venueFilter, setVenueFilter] = useState<Venue | ''>('')
  const [statusFilter, setStatusFilter] = useState<MarketStatus | ''>('')
  const { data, isLoading, error } = useGroups(minConfidence || undefined)

  const filteredGroups = useMemo(() => {
    if (!data?.groups) return []
    return data.groups.filter((g) => {
      if (venueFilter && !g.members?.some((m) => m.venue === venueFilter))
        return false
      if (statusFilter && !g.members?.some((m) => m.status === statusFilter))
        return false
      return true
    })
  }, [data?.groups, venueFilter, statusFilter])

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">
          Equivalence Groups
        </h1>
        <span className="text-sm text-gray-500">
          Auto-refreshes every 30s
        </span>
      </div>

      <div className="mb-5 flex flex-wrap items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 sm:gap-4">
        <div className="flex items-center gap-2">
          <label className="text-xs font-medium text-gray-600">
            Min Confidence
          </label>
          <input
            type="range"
            min={0}
            max={100}
            value={minConfidence * 100}
            onChange={(e) => setMinConfidence(Number(e.target.value) / 100)}
            className="w-24"
          />
          <span className="w-10 text-xs text-gray-500">
            {(minConfidence * 100).toFixed(0)}%
          </span>
        </div>
        <select
          value={venueFilter}
          onChange={(e) => setVenueFilter(e.target.value as Venue | '')}
          className="rounded-md border border-gray-300 px-2 py-1 text-xs"
        >
          <option value="">All Venues</option>
          <option value="KALSHI">Kalshi</option>
          <option value="POLYMARKET">Polymarket</option>
        </select>
        <select
          value={statusFilter}
          onChange={(e) =>
            setStatusFilter(e.target.value as MarketStatus | '')
          }
          className="rounded-md border border-gray-300 px-2 py-1 text-xs"
        >
          <option value="">All Statuses</option>
          <option value="OPEN">Open</option>
          <option value="CLOSED">Closed</option>
          <option value="RESOLVED">Resolved</option>
        </select>
      </div>

      {isLoading && (
        <div className="py-12 text-center text-sm text-gray-500">
          Loading groups...
        </div>
      )}
      {error && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">
          Failed to load groups: {(error as Error).message}
        </div>
      )}
      {!isLoading && filteredGroups.length === 0 && (
        <div className="rounded-lg border border-dashed border-gray-300 px-6 py-12 text-center">
          <p className="text-sm text-gray-500">
            No equivalence groups found.
          </p>
          <p className="mt-1 text-xs text-gray-400">
            Run{' '}
            <code className="rounded bg-gray-100 px-1">
              equinox ingest && equinox match
            </code>{' '}
            to populate.
          </p>
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
        {filteredGroups.map((group) => (
          <GroupCard key={group.group_id} group={group} />
        ))}
      </div>
    </div>
  )
}
