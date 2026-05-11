import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/app/settings/connectors')({
  component: ConnectorsLayout,
})

function ConnectorsLayout() {
  return <Outlet />
}
