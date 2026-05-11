import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/app/settings/connectors/external')({
  component: ExternalConnectorsLayout,
})

function ExternalConnectorsLayout() {
  return <Outlet />
}
