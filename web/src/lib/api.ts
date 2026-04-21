export type DataType =
  | 'text'
  | 'integer'
  | 'bigint'
  | 'numeric'
  | 'boolean'
  | 'date'
  | 'timestamptz'
  | 'uuid'
  | 'jsonb'
  | 'reference'

export interface User {
  id: string
  email: string
  name: string
  email_verified_at: string | null
  created_at: string
}

export interface Membership {
  id: string
  org_id: string
  org_slug: string
  org_name: string
  role: 'owner' | 'admin' | 'member'
}

export interface Field {
  id: string
  name: string
  display_name: string
  data_type: DataType
  is_required: boolean
  is_unique: boolean
  reference_entity?: string
}

export interface Entity {
  id: string
  name: string
  display_name: string
  description?: string
  fields: Field[]
  created_at: string
}

export interface MeResponse {
  user: User
  memberships: Membership[]
  active_membership: Membership | null
}

export interface RefOption {
  ID: string
  Label: string
}

export type WidgetType = 'kpi' | 'bar' | 'line' | 'area' | 'pie' | 'table'

export interface QuerySpec {
  entity: string
  filters?: {
    field: string
    op: string
    value?: unknown
  }[]
  group_by?: { field: string; bucket?: '' | 'day' | 'week' | 'month' | 'quarter' | 'year' }
  series_by?: { field: string; bucket?: '' | 'day' | 'week' | 'month' | 'quarter' | 'year' }
  aggregate?: { fn: 'count' | 'sum' | 'avg' | 'min' | 'max'; field?: string }
  sort?: { field: string; dir: 'asc' | 'desc' }
  limit?: number
  date_filter_field?: string
  compare_period?: '' | 'previous_period' | 'previous_year'
}

export interface ReportOptions {
  stacked?: boolean
  number_format?: 'integer' | 'decimal' | 'currency' | 'percent'
  currency_code?: string
  locale?: string
}

export interface Report {
  id: string
  dashboard_id: string
  title: string
  subtitle?: string
  widget_type: WidgetType
  query_spec: QuerySpec
  options?: ReportOptions
  width: number
  position: number
  created_at: string
  updated_at: string
}

export interface Dashboard {
  id: string
  tenant_id: string
  name: string
  slug: string
  description?: string
  position: number
  reports?: Report[]
  created_at: string
  updated_at: string
}

export interface ReportResult {
  shape: 'kpi' | 'series' | 'table'
  columns?: string[]
  rows: Record<string, unknown>[]
}

export interface LLMProvider {
  id: string
  name: string
  base_url: string
  default_model: string
  requires_api_key: boolean
  local?: boolean
  notes?: string
}

export interface LLMConfigSafe {
  provider?: string
  base_url?: string
  model?: string
  has_api_key?: boolean
  source?: 'tenant' | 'env-fallback' | ''
  updated_at?: string
}

export interface LLMTestResult {
  ok: boolean
  models_ok: boolean
  chat_ok: boolean
  tools_ok: boolean
  message?: string
  model?: string
}

export interface RowsResponse {
  entity: Entity
  rows: Record<string, unknown>[]
  ref_options: Record<string, RefOption[]>
  total: number
  page: number
  limit: number
}

export interface ListRowsParams {
  sort?: string
  dir?: 'asc' | 'desc'
  page?: number
  limit?: number
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
    ...init,
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    const msg = (body && (body.error as string)) || `HTTP ${res.status}`
    throw new ApiError(res.status, msg)
  }
  return body as T
}

export const api = {
  signup: (body: {
    email: string
    name: string
    password: string
    org_name?: string
    org_slug?: string
  }) => request<{ user: User; active_org_id: string | null }>('/api/v1/auth/signup', {
    method: 'POST',
    body: JSON.stringify(body),
  }),

  login: (body: { email: string; password: string }) =>
    request<{ user: User }>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),

  forgotPassword: (email: string) =>
    request<void>('/api/v1/auth/forgot', {
      method: 'POST',
      body: JSON.stringify({ email }),
    }),

  resetPassword: (token: string, password: string) =>
    request<void>('/api/v1/auth/reset', {
      method: 'POST',
      body: JSON.stringify({ token, password }),
    }),

  me: () => request<MeResponse>('/api/v1/me'),

  createOrg: (body: { name: string; slug: string }) =>
    request<{ membership: Membership }>('/api/v1/orgs', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  activateMembership: (id: string) =>
    request<{ active_membership: Membership }>(
      `/api/v1/memberships/${id}/activate`,
      { method: 'POST' }
    ),

  listEntities: () =>
    request<{ entities: Entity[] }>('/api/v1/entities').then((r) => r.entities),

  proposeEntity: (description: string) =>
    request<{ entity: Entity }>('/api/v1/entities', {
      method: 'POST',
      body: JSON.stringify({ description }),
    }).then((r) => r.entity),

  getEntity: (name: string) =>
    request<{ entity: Entity }>(`/api/v1/entities/${encodeURIComponent(name)}`).then(
      (r) => r.entity
    ),

  listRows: (name: string, params: ListRowsParams = {}) => {
    const qs = new URLSearchParams()
    if (params.sort) qs.set('sort', params.sort)
    if (params.dir) qs.set('dir', params.dir)
    if (params.page) qs.set('page', String(params.page))
    if (params.limit) qs.set('limit', String(params.limit))
    const suffix = qs.toString() ? `?${qs}` : ''
    return request<RowsResponse>(
      `/api/v1/entities/${encodeURIComponent(name)}/rows${suffix}`
    )
  },

  createRow: (name: string, values: Record<string, string>) =>
    request<{ id: string }>(
      `/api/v1/entities/${encodeURIComponent(name)}/rows`,
      { method: 'POST', body: JSON.stringify({ values }) }
    ),

  deleteRow: (name: string, id: string) =>
    request<void>(
      `/api/v1/entities/${encodeURIComponent(name)}/rows/${encodeURIComponent(id)}`,
      { method: 'DELETE' }
    ),

  updateRow: (name: string, id: string, values: Record<string, string>) =>
    request<void>(
      `/api/v1/entities/${encodeURIComponent(name)}/rows/${encodeURIComponent(id)}`,
      { method: 'PATCH', body: JSON.stringify({ values }) }
    ),

  addField: (name: string, field: {
    name: string
    display_name: string
    data_type: DataType
    is_required?: boolean
    is_unique?: boolean
    reference_entity?: string
  }) =>
    request<{ entity: Entity }>(
      `/api/v1/entities/${encodeURIComponent(name)}/fields`,
      { method: 'POST', body: JSON.stringify(field) }
    ),

  dropField: (name: string, field: string) =>
    request<void>(
      `/api/v1/entities/${encodeURIComponent(name)}/fields/${encodeURIComponent(field)}`,
      { method: 'DELETE' }
    ),

  listFieldOptions: (entityName: string, fieldName: string) =>
    request<{ options: RefOption[] }>(
      `/api/v1/entities/${encodeURIComponent(entityName)}/fields/${encodeURIComponent(fieldName)}/options`
    ).then((r) => r.options),

  listTemplates: () =>
    request<{ templates: { id: string; name: string; description: string }[] }>(
      '/api/v1/templates'
    ).then((r) => r.templates),

  applyTemplate: (id: string) =>
    request<void>(`/api/v1/templates/${encodeURIComponent(id)}/apply`, {
      method: 'POST',
    }),

  listDashboards: () =>
    request<{ dashboards: Dashboard[] }>('/api/v1/dashboards').then((r) => r.dashboards),

  getDashboard: (slug: string) =>
    request<{ dashboard: Dashboard }>(`/api/v1/dashboards/${encodeURIComponent(slug)}`).then(
      (r) => r.dashboard
    ),

  deleteDashboard: (slug: string) =>
    request<void>(`/api/v1/dashboards/${encodeURIComponent(slug)}`, { method: 'DELETE' }),

  createDashboard: (body: { name: string; slug?: string; description?: string }) =>
    request<{ dashboard: Dashboard }>('/api/v1/dashboards', {
      method: 'POST',
      body: JSON.stringify(body),
    }).then((r) => r.dashboard),

  updateDashboard: (slug: string, body: { name?: string; description?: string }) =>
    request<{ dashboard: Dashboard }>(`/api/v1/dashboards/${encodeURIComponent(slug)}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }).then((r) => r.dashboard),

  updateReport: (id: string, body: {
    title?: string
    subtitle?: string
    widget_type?: WidgetType
    width?: number
    query_spec?: QuerySpec
    options?: ReportOptions
  }) =>
    request<void>(`/api/v1/reports/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),

  llmProviders: () =>
    request<{ providers: LLMProvider[] }>('/api/v1/llm/providers').then((r) => r.providers),

  llmConfig: () =>
    request<{ config: LLMConfigSafe }>('/api/v1/llm/config').then((r) => r.config),

  putLLMConfig: (body: {
    provider: string
    base_url: string
    api_key?: string | null
    model: string
  }) =>
    request<{ config: LLMConfigSafe }>('/api/v1/llm/config', {
      method: 'PUT',
      body: JSON.stringify(body),
    }).then((r) => r.config),

  deleteLLMConfig: () =>
    request<void>('/api/v1/llm/config', { method: 'DELETE' }),

  llmListModels: (body: { base_url: string; api_key: string }) =>
    request<{ models: { id: string }[] }>('/api/v1/llm/models/list', {
      method: 'POST',
      body: JSON.stringify(body),
    }).then((r) => r.models),

  llmTest: (body: { base_url: string; api_key: string; model: string }) =>
    request<{ result: LLMTestResult }>('/api/v1/llm/test', {
      method: 'POST',
      body: JSON.stringify(body),
    }).then((r) => r.result),

  reorderReports: (slug: string, reportIDs: string[]) =>
    request<void>(
      `/api/v1/dashboards/${encodeURIComponent(slug)}/reports/reorder`,
      { method: 'POST', body: JSON.stringify({ report_ids: reportIDs }) }
    ),

  addReport: (slug: string, body: {
    title: string
    subtitle?: string
    widget_type: WidgetType
    query_spec: QuerySpec
    options?: ReportOptions
    width?: number
  }) =>
    request<{ report: Report }>(
      `/api/v1/dashboards/${encodeURIComponent(slug)}/reports`,
      { method: 'POST', body: JSON.stringify(body) }
    ).then((r) => r.report),

  executeReport: (id: string, range?: { from?: string; to?: string }) => {
    const qs = new URLSearchParams()
    if (range?.from) qs.set('from', range.from)
    if (range?.to) qs.set('to', range.to)
    const suffix = qs.toString() ? `?${qs}` : ''
    return request<{ result: ReportResult }>(
      `/api/v1/reports/${encodeURIComponent(id)}/execute${suffix}`,
      { method: 'POST' }
    ).then((r) => r.result)
  },

  deleteReport: (id: string) =>
    request<void>(`/api/v1/reports/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  chat: (body: { history: { role: string; text: string }[]; message: string }) =>
    request<{
      assistant: {
        role: 'assistant'
        text: string
        actions?: {
          tool: string
          input: unknown
          summary: string
          entity_name?: string
          error?: string
        }[]
      }
    }>('/api/v1/chat/messages', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
}
