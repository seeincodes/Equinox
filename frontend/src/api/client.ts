import type {
  User,
  Market,
  EquivalenceGroup,
  RoutingDecision,
  PriceSnapshot,
  ConfigMap,
} from '../lib/types'

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })

  if (res.status === 401) {
    if (!window.location.pathname.startsWith('/login')) {
      window.location.href = '/login'
    }
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new ApiError(res.status, body.error || res.statusText)
  }

  return res.json()
}

export const api = {
  // Auth
  login: (email: string, password: string) =>
    request<{ status: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),
  logout: () => request<{ status: string }>('/auth/logout', { method: 'POST' }),
  me: () => request<User>('/auth/me'),

  // Markets
  getMarkets: (params?: { venue?: string; status?: string }) => {
    const qs = params
      ? new URLSearchParams(
          Object.fromEntries(Object.entries(params).filter(([, v]) => v)),
        ).toString()
      : ''
    return request<{ markets: Market[] | null; count: number }>(
      `/markets${qs ? `?${qs}` : ''}`,
    )
  },

  // Groups
  getGroups: (minConfidence?: number) => {
    const qs =
      minConfidence != null ? `?min_confidence=${minConfidence}` : ''
    return request<{ groups: EquivalenceGroup[] | null; count: number }>(
      `/groups${qs}`,
    )
  },

  getGroupHistory: (groupId: string) =>
    request<{
      group_id: string
      history: PriceSnapshot[] | null
      count: number
    }>(`/groups/${groupId}/history`),

  // Routing
  route: (body: {
    market_id?: string
    group_id?: string
    side: string
    size: number
  }) =>
    request<RoutingDecision>('/route', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  // Decisions
  getDecisions: (params?: {
    venue?: string
    after?: string
    page?: number
    per_page?: number
    user?: string
  }) => {
    const qs = params
      ? new URLSearchParams(
          Object.fromEntries(
            Object.entries(params)
              .filter(([, v]) => v != null)
              .map(([k, v]) => [k, String(v)]),
          ),
        ).toString()
      : ''
    return request<{
      decisions: RoutingDecision[] | null
      count: number
      total_count: number
      page: number
      per_page: number
    }>(`/decisions${qs ? `?${qs}` : ''}`)
  },

  // Config
  getConfig: () => request<ConfigMap>('/config'),
  putConfig: (config: ConfigMap) =>
    request<ConfigMap>('/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),
}
