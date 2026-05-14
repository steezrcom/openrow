import { createFileRoute, notFound } from '@tanstack/react-router'
import { DocsConnectorPage } from '@/components/DocsConnectorPage'
import { docsConnectors } from '@/content/docs/connectors'

export const Route = createFileRoute('/docs/connectors/$id')({
  loader: ({ params }) => {
    const c = docsConnectors.find((c) => c.id === params.id)
    if (!c) throw notFound()
    return { connector: c }
  },
  component: ConnectorPage,
  notFoundComponent: NotFound,
})

function ConnectorPage() {
  const { connector } = Route.useLoaderData()
  return <DocsConnectorPage c={connector} />
}

function NotFound() {
  return (
    <div className="py-16 text-center">
      <p className="text-2xl font-semibold">Unknown connector</p>
      <p className="mt-2 text-sm text-muted-foreground">
        Check the connectors index at /docs/connectors.
      </p>
    </div>
  )
}
