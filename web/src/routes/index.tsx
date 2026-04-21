import { createFileRoute, redirect } from '@tanstack/react-router'
import { api, ApiError } from '@/lib/api'

export const Route = createFileRoute('/')({
  beforeLoad: async ({ context }) => {
    try {
      const me = await context.queryClient.fetchQuery({
        queryKey: ['me'],
        queryFn: api.me,
        staleTime: 10_000,
      })
      if (me.active_membership) throw redirect({ to: '/app' })
      throw redirect({ to: '/orgs' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: '/login' })
      }
      throw err
    }
  },
})
