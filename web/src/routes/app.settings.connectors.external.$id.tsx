import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/app/settings/connectors/external/$id')({
  component: ExternalBindingDetail,
})

function ExternalBindingDetail() {
  const { id } = Route.useParams()
  return <div className="px-8 py-10 text-sm text-muted-foreground">Binding {id}</div>
}
