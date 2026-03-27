import { useState } from 'react'
import { api } from '../api/client'
import type { RoutingDecision } from '../lib/types'
import { ScoringBreakdown } from './scoring-breakdown'

interface Props {
  groupId: string
}

export function RoutingPanel({ groupId }: Props) {
  const [side, setSide] = useState<'YES' | 'NO'>('YES')
  const [size, setSize] = useState(100)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<RoutingDecision | null>(null)
  const [error, setError] = useState('')

  const handleRoute = async () => {
    setLoading(true)
    setError('')
    setResult(null)
    try {
      const decision = await api.route({ group_id: groupId, side, size })
      setResult(decision)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="mb-3 text-sm font-medium text-gray-700">
          Simulate Route
        </h3>
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label className="block text-xs text-gray-500">Side</label>
            <div className="mt-1 flex rounded-md border border-gray-300">
              <button
                onClick={() => setSide('YES')}
                className={`px-3 py-1.5 text-xs font-medium ${
                  side === 'YES'
                    ? 'bg-green-600 text-white'
                    : 'text-gray-600 hover:bg-gray-50'
                } rounded-l-md`}
              >
                YES
              </button>
              <button
                onClick={() => setSide('NO')}
                className={`px-3 py-1.5 text-xs font-medium ${
                  side === 'NO'
                    ? 'bg-red-600 text-white'
                    : 'text-gray-600 hover:bg-gray-50'
                } rounded-r-md`}
              >
                NO
              </button>
            </div>
          </div>
          <div>
            <label className="block text-xs text-gray-500">Size</label>
            <input
              type="number"
              min={1}
              value={size}
              onChange={(e) => setSize(Number(e.target.value))}
              className="mt-1 w-24 rounded-md border border-gray-300 px-2 py-1.5 text-sm"
            />
          </div>
          <button
            onClick={handleRoute}
            disabled={loading}
            className="rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {loading ? 'Routing...' : 'Route'}
          </button>
        </div>
        {error && (
          <div className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        )}
      </div>

      {result && (
        <div className="space-y-3">
          <div className="rounded-lg border border-green-200 bg-green-50 p-4">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-xs font-medium text-green-600">
                  SELECTED
                </span>
                <p className="text-lg font-semibold text-green-900">
                  {result.selected_venue}
                </p>
              </div>
              <div className="text-right">
                <span className="text-xs text-green-600">Score</span>
                <p className="text-lg font-bold text-green-900">
                  {result.scoring_breakdown[result.selected_venue]?.total.toFixed(
                    4,
                  )}
                </p>
              </div>
            </div>
          </div>

          <ScoringBreakdown
            breakdown={result.scoring_breakdown}
            selectedVenue={result.selected_venue}
          />

          {result.rejected_alternatives?.length > 0 && (
            <div className="rounded-lg border border-gray-200 bg-white p-4">
              <h4 className="mb-2 text-xs font-medium text-gray-500">
                REJECTED
              </h4>
              {result.rejected_alternatives.map((r) => (
                <div
                  key={r.market_id}
                  className="flex items-center justify-between text-sm"
                >
                  <span className="text-gray-600">{r.venue}</span>
                  <span className="text-xs text-gray-400">
                    {r.rejection_reason}
                  </span>
                </div>
              ))}
            </div>
          )}

          <details className="rounded-lg border border-gray-200 bg-white">
            <summary className="cursor-pointer px-4 py-2 text-xs font-medium text-gray-500">
              Routing Rationale
            </summary>
            <pre className="whitespace-pre-wrap px-4 py-3 font-mono text-xs text-gray-600">
              {result.routing_rationale}
            </pre>
          </details>
        </div>
      )}
    </div>
  )
}
