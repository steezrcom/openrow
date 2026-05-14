import { createFileRoute, redirect } from '@tanstack/react-router'
import { api, ApiError } from '@/lib/api'
import { MarketingHome } from '@/components/MarketingHome'

// Root route. Logged-in users with an active workspace go straight to
// /app; logged-in users without a workspace go to /orgs to pick one.
// Anonymous visitors get the marketing page below.
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
        // Anonymous — render the marketing page.
        return
      }
      throw err
    }
  },
  component: MarketingHome,
})
