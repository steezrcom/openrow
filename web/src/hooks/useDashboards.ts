import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

export function useDashboards() {
  return useQuery({
    queryKey: ['dashboards'],
    queryFn: api.listDashboards,
  })
}

export function useDashboard(slug: string) {
  return useQuery({
    queryKey: ['dashboard', slug],
    queryFn: () => api.getDashboard(slug),
  })
}
