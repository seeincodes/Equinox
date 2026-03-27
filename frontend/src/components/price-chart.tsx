import { useMemo, useState } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts'
import type { PriceSnapshot } from '../lib/types'

interface Props {
  history: PriceSnapshot[] | null
}

const timeRanges = [
  { label: '1h', hours: 1 },
  { label: '6h', hours: 6 },
  { label: '24h', hours: 24 },
  { label: '7d', hours: 168 },
  { label: 'All', hours: Infinity },
]

const venueColors: Record<string, string> = {
  KALSHI: '#3B82F6',
  POLYMARKET: '#8B5CF6',
}

export function PriceChart({ history }: Props) {
  const [range, setRange] = useState(168) // default 7d

  const chartData = useMemo(() => {
    if (!history || history.length === 0) return []

    const cutoff =
      range === Infinity
        ? 0
        : Date.now() - range * 60 * 60 * 1000

    const filtered = history
      .filter((s) => new Date(s.timestamp).getTime() >= cutoff)
      .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

    // Group by timestamp and pivot venues into columns
    const byTime = new Map<string, Record<string, number>>()
    for (const s of filtered) {
      const ts = s.timestamp
      if (!byTime.has(ts)) byTime.set(ts, { time: new Date(ts).getTime() })
      byTime.get(ts)![s.venue] = s.yes_price * 100
    }

    return Array.from(byTime.values())
  }, [history, range])

  const venues = useMemo(() => {
    if (!history) return []
    return [...new Set(history.map((s) => s.venue))]
  }, [history])

  if (!history || history.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-gray-300 px-6 py-8 text-center text-sm text-gray-500">
        No price history available. Run normalization to capture snapshots.
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium text-gray-700">
          YES Price History
        </h3>
        <div className="flex gap-1">
          {timeRanges.map((tr) => (
            <button
              key={tr.label}
              onClick={() => setRange(tr.hours)}
              className={`rounded px-2 py-1 text-xs font-medium ${
                range === tr.hours
                  ? 'bg-blue-100 text-blue-700'
                  : 'text-gray-500 hover:bg-gray-100'
              }`}
            >
              {tr.label}
            </button>
          ))}
        </div>
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis
            dataKey="time"
            type="number"
            domain={['dataMin', 'dataMax']}
            tickFormatter={(v) =>
              new Date(v).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
              })
            }
            tick={{ fontSize: 11 }}
          />
          <YAxis
            domain={[0, 100]}
            tick={{ fontSize: 11 }}
            tickFormatter={(v) => `${v}%`}
          />
          <Tooltip
            labelFormatter={(v) => new Date(v as number).toLocaleString()}
            formatter={(v) => [`${Number(v).toFixed(1)}%`]}
          />
          <Legend />
          {venues.map((venue) => (
            <Line
              key={venue}
              type="monotone"
              dataKey={venue}
              stroke={venueColors[venue] ?? '#888'}
              strokeWidth={2}
              dot={false}
              name={venue}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
