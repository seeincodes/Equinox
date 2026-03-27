import { useState } from 'react'
import { useDecisions } from '../hooks/use-decisions'
import { ScoringBreakdown } from '../components/scoring-breakdown'
import { formatScore } from '../lib/utils'
import type { RoutingDecision } from '../lib/types'

export function DecisionsPage() {
  const [page, setPage] = useState(1)
  const [venueFilter, setVenueFilter] = useState('')
  const [expanded, setExpanded] = useState<string | null>(null)

  const { data, isLoading, error } = useDecisions({
    page,
    per_page: 20,
    venue: venueFilter || undefined,
  })

  const decisions = data?.decisions ?? []
  const totalCount = data?.total_count ?? 0
  const totalPages = Math.ceil(totalCount / 20)

  return (
    <div>
      <h1 className="mb-6 text-xl font-semibold text-gray-900">
        Routing Decisions
      </h1>

      <div className="mb-4 flex items-center gap-3">
        <select
          value={venueFilter}
          onChange={(e) => {
            setVenueFilter(e.target.value)
            setPage(1)
          }}
          className="rounded-md border border-gray-300 px-2 py-1.5 text-sm"
        >
          <option value="">All Venues</option>
          <option value="KALSHI">Kalshi</option>
          <option value="POLYMARKET">Polymarket</option>
        </select>
        <span className="text-sm text-gray-500">
          {totalCount} total decisions
        </span>
      </div>

      {isLoading && (
        <div className="py-8 text-center text-sm text-gray-500">
          Loading...
        </div>
      )}
      {error && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
        <table className="w-full min-w-[600px] text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500">
                Time
              </th>
              <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500">
                Market
              </th>
              <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500">
                Selected
              </th>
              <th className="px-4 py-2.5 text-right text-xs font-medium text-gray-500">
                Score
              </th>
              <th className="px-4 py-2.5 text-right text-xs font-medium text-gray-500">
                Order
              </th>
            </tr>
          </thead>
          <tbody>
            {decisions.map((d: RoutingDecision) => (
              <DecisionRow
                key={d.decision_id}
                decision={d}
                expanded={expanded === d.decision_id}
                onToggle={() =>
                  setExpanded(
                    expanded === d.decision_id ? null : d.decision_id,
                  )
                }
              />
            ))}
            {!isLoading && decisions.length === 0 && (
              <tr>
                <td
                  colSpan={5}
                  className="px-4 py-8 text-center text-sm text-gray-500"
                >
                  No decisions found
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="mt-4 flex items-center justify-between">
          <button
            disabled={page <= 1}
            onClick={() => setPage(page - 1)}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50"
          >
            Previous
          </button>
          <span className="text-sm text-gray-500">
            Page {page} of {totalPages}
          </span>
          <button
            disabled={page >= totalPages}
            onClick={() => setPage(page + 1)}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50"
          >
            Next
          </button>
        </div>
      )}
    </div>
  )
}

function DecisionRow({
  decision,
  expanded,
  onToggle,
}: {
  decision: RoutingDecision
  expanded: boolean
  onToggle: () => void
}) {
  const score =
    decision.scoring_breakdown[decision.selected_venue]?.total ?? 0

  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer border-t border-gray-100 hover:bg-gray-50"
      >
        <td className="px-4 py-2.5 text-xs text-gray-500">
          {new Date(decision.created_at).toLocaleString()}
        </td>
        <td className="px-4 py-2.5 text-gray-700">
          {decision.selected_market_id.split(':')[1]?.slice(0, 20) ?? '—'}
        </td>
        <td className="px-4 py-2.5">
          <span className="rounded-full bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">
            {decision.selected_venue}
          </span>
        </td>
        <td className="px-4 py-2.5 text-right font-mono text-gray-700">
          {formatScore(score)}
        </td>
        <td className="px-4 py-2.5 text-right text-xs text-gray-500">
          {decision.order_request.size} {decision.order_request.side}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={5} className="border-t border-gray-100 bg-gray-50 p-4">
            <div className="space-y-3">
              <ScoringBreakdown
                breakdown={decision.scoring_breakdown}
                selectedVenue={decision.selected_venue}
              />
              <details className="rounded border border-gray-200 bg-white">
                <summary className="cursor-pointer px-3 py-2 text-xs font-medium text-gray-500">
                  Routing Rationale
                </summary>
                <pre className="whitespace-pre-wrap px-3 py-2 font-mono text-xs text-gray-600">
                  {decision.routing_rationale}
                </pre>
              </details>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
