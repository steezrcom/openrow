import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

export function useFieldOptions(entityName: string | undefined, fieldName: string | undefined) {
  return useQuery({
    queryKey: ['field-options', entityName ?? '', fieldName ?? ''],
    queryFn: () => api.listFieldOptions(entityName!, fieldName!),
    enabled: Boolean(entityName && fieldName),
    staleTime: 60_000,
  })
}
