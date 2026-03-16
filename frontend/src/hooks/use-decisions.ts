import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

export function useDecisions(params?: {
  venue?: string
  after?: string
  page?: number
  per_page?: number
  user?: string
}) {
  return useQuery({
    queryKey: ['decisions', params],
    queryFn: () => api.getDecisions(params),
  })
}
