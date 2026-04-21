import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

export function useEntityDetail(name: string | undefined) {
  return useQuery({
    queryKey: ['entity-detail', name ?? ''],
    queryFn: () => api.getEntity(name!),
    enabled: Boolean(name),
    staleTime: 30_000,
  })
}
