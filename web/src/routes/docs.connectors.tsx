import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/docs/connectors')({
  component: ConnectorsRoute,
})

function ConnectorsRoute() {
  return <Outlet />
}
