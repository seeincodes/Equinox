import { useState, useEffect } from 'react'
import { useConfig, useUpdateConfig } from '../hooks/use-config'
import { useAuth } from '../hooks/use-auth'
import { Navigate } from 'react-router'

const defaultConfig = {
  weight_price_quality: '0.40',
  weight_liquidity: '0.35',
  weight_spread_quality: '0.15',
  weight_market_status: '0.10',
  staleness_liquidity_haircut: '0.20',
  threshold_high_confidence: '0.92',
  threshold_medium_confidence: '0.78',
}

export function SettingsPage() {
  const { user } = useAuth()
  const { data: config, isLoading } = useConfig()
  const updateConfig = useUpdateConfig()
  const [values, setValues] = useState(defaultConfig)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (config) {
      setValues((prev) => ({ ...prev, ...config }))
    }
  }, [config])

  if (user?.role !== 'admin') return <Navigate to="/" />

  const weightKeys = [
    'weight_price_quality',
    'weight_liquidity',
    'weight_spread_quality',
    'weight_market_status',
  ] as const
  const weightSum = weightKeys.reduce(
    (sum, k) => sum + parseFloat(values[k] || '0'),
    0,
  )
  const weightsValid = Math.abs(weightSum - 1.0) < 0.001

  const high = parseFloat(values.threshold_high_confidence || '0')
  const med = parseFloat(values.threshold_medium_confidence || '0')
  const thresholdsValid = high > med

  const canSave = weightsValid && thresholdsValid

  const handleSave = async () => {
    setSaved(false)
    await updateConfig.mutateAsync(values)
    setSaved(true)
    setTimeout(() => setSaved(false), 3000)
  }

  const handleReset = () => setValues(defaultConfig)

  if (isLoading) {
    return (
      <div className="py-8 text-center text-sm text-gray-500">
        Loading config...
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <h1 className="text-xl font-semibold text-gray-900">Settings</h1>

      <div className="rounded-lg border border-gray-200 bg-white p-5">
        <h2 className="mb-4 text-sm font-medium text-gray-700">
          Routing Weights
        </h2>
        <div className="space-y-4">
          {weightKeys.map((key) => (
            <div key={key} className="flex items-center gap-3">
              <label className="w-40 text-sm text-gray-600">
                {key
                  .replace('weight_', '')
                  .replace(/_/g, ' ')
                  .replace(/\b\w/g, (c) => c.toUpperCase())}
              </label>
              <input
                type="range"
                min={0}
                max={100}
                value={parseFloat(values[key]) * 100}
                onChange={(e) =>
                  setValues({
                    ...values,
                    [key]: (Number(e.target.value) / 100).toFixed(2),
                  })
                }
                className="flex-1"
              />
              <span className="w-12 text-right font-mono text-sm text-gray-700">
                {values[key]}
              </span>
            </div>
          ))}
        </div>
        <div className="mt-3 flex items-center gap-2">
          <span className="text-xs text-gray-500">Sum:</span>
          <span
            className={`font-mono text-sm font-medium ${weightsValid ? 'text-green-600' : 'text-red-600'}`}
          >
            {weightSum.toFixed(2)}
          </span>
          {!weightsValid && (
            <span className="text-xs text-red-500">Must equal 1.00</span>
          )}
        </div>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-5">
        <h2 className="mb-4 text-sm font-medium text-gray-700">
          Confidence Thresholds (Display)
        </h2>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm text-gray-600">
              HIGH threshold
            </label>
            <input
              type="number"
              step={0.01}
              min={0}
              max={1}
              value={values.threshold_high_confidence}
              onChange={(e) =>
                setValues({
                  ...values,
                  threshold_high_confidence: e.target.value,
                })
              }
              className="mt-1 w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm"
            />
          </div>
          <div>
            <label className="block text-sm text-gray-600">
              MEDIUM threshold
            </label>
            <input
              type="number"
              step={0.01}
              min={0}
              max={1}
              value={values.threshold_medium_confidence}
              onChange={(e) =>
                setValues({
                  ...values,
                  threshold_medium_confidence: e.target.value,
                })
              }
              className="mt-1 w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm"
            />
          </div>
        </div>
        {!thresholdsValid && (
          <p className="mt-2 text-xs text-red-500">
            HIGH must be greater than MEDIUM
          </p>
        )}
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-5">
        <h2 className="mb-4 text-sm font-medium text-gray-700">
          Staleness Settings
        </h2>
        <div>
          <label className="block text-sm text-gray-600">
            Liquidity Haircut (%)
          </label>
          <input
            type="number"
            step={1}
            min={0}
            max={100}
            value={parseFloat(values.staleness_liquidity_haircut) * 100}
            onChange={(e) =>
              setValues({
                ...values,
                staleness_liquidity_haircut: (
                  Number(e.target.value) / 100
                ).toFixed(2),
              })
            }
            className="mt-1 w-32 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
          />
        </div>
      </div>

      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={!canSave || updateConfig.isPending}
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {updateConfig.isPending ? 'Saving...' : 'Save Changes'}
        </button>
        <button
          onClick={handleReset}
          className="rounded-md border border-gray-300 px-4 py-2 text-sm text-gray-600 hover:bg-gray-50"
        >
          Reset to Defaults
        </button>
        {saved && (
          <span className="text-sm text-green-600">Saved successfully</span>
        )}
        {updateConfig.isError && (
          <span className="text-sm text-red-600">
            {(updateConfig.error as Error).message}
          </span>
        )}
      </div>
    </div>
  )
}
