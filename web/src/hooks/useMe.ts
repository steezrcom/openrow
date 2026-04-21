import { useQuery } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: api.me,
    retry: (count, err) => {
      if (err instanceof ApiError && err.status === 401) return false
      return count < 2
    },
  })
}
