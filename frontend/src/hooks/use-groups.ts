import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

export function useGroups(minConfidence?: number) {
  return useQuery({
    queryKey: ['groups', minConfidence],
    queryFn: () => api.getGroups(minConfidence),
    refetchInterval: 30_000,
  })
}

export function useGroupHistory(groupId: string) {
  return useQuery({
    queryKey: ['group-history', groupId],
    queryFn: () => api.getGroupHistory(groupId),
    enabled: !!groupId,
  })
}
