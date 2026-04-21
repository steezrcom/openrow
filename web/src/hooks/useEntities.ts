import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

export function useEntities() {
  return useQuery({
    queryKey: ['entities'],
    queryFn: api.listEntities,
  })
}
