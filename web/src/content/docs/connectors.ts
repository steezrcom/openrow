// Hand-authored docs metadata for every built-in connector. Source of
// truth for shape lives in internal/connectors/catalog; the prose here is
// what we show end users on /docs/connectors/<id>.

export type DocsConnectorField = {
  label: string
  required?: boolean
  secret?: boolean
  placeholder?: string
  description: string
}

export type DocsConnectorAction = {
  name: string
  description: string
  mutates?: boolean
}

export type DocsConnectorPhase = {
  title: string
  blurb?: string
  steps: string[]
}

export type DocsConnector = {
  id: string
  name: string
  category: string
  status: 'available' | 'coming_soon'
  homepage: string
  short: string
  intro: string
  steps: string[]
  phases?: DocsConnectorPhase[]
  fields: DocsConnectorField[]
  actions: DocsConnectorAction[]
  webhook?: string
  example?: string
  references?: { label: string; url: string }[]
}

export const docsConnectors: DocsConnector[] = [
  // --- Banking ---------------------------------------------------------
  {
    id: 'csas',
    name: 'Česká spořitelna',
    category: 'banking',
    status: 'available',
    homepage: 'https://developers.erstegroup.com',
    short: 'Read own-account transactions via the George Developer API.',
    intro:
      "Read-only access to your Česká spořitelna business accounts through Erste Group's George Developer API. The full onboarding has two halves: a sandbox you can verify in a single sitting, and a production application the bank reviews before issuing live credentials. Plan for the production review to take a few business days.",
    steps: [],
    phases: [
      {
        title: '1. Register on the Erste Developer Portal',
        blurb:
          'You need a developer account before you can do anything else. The portal is shared across Erste Group banks, but the credentials you get are scoped to whichever bank you connect.',
        steps: [
          'Open developers.erstegroup.com and create an account. Use a work address you can keep for the long run; bank approvals are tied to this identity.',
          "From the dashboard, create a new application. The portal asks for a name, a short description, and an icon. Pick the application type 'Final API Consumer' (you're managing your own accounts, not building a service that handles other companies' accounts on their behalf). Save.",
        ],
      },
      {
        title: '2. Connect Česká spořitelna and pick the Accounts API',
        blurb:
          'A fresh application has no banks attached. You add Česká spořitelna explicitly and configure which APIs you want from it.',
        steps: [
          "Open the application overview. Add a new bank connection and pick 'Česká spořitelna a.s.' from the list.",
          "Inside that bank connection, add the Accounts API product. This is the one that exposes balances and transactions.",
          "Open the Scopes tab and tick siblings.accounts. That scope is what lets the API see your own accounts; without it the token works but every call returns empty.",
        ],
      },
      {
        title: '3. Configure OAuth 2.0',
        blurb:
          "The Accounts API uses the authorization-code flow with a long-lived refresh token. You set this up once and openrow reuses the refresh token forever (rotating it on every successful refresh).",
        steps: [
          'Enable OAuth 2.0 on the bank connection.',
          "Set the grant type to Code (authorization code). Don't pick Implicit; openrow needs a refresh token to keep working without re-consent.",
          "Add a redirect URI you control. This is where the bank sends the user after consent. A small static page on your own domain works; so does a localhost handler you run for the one-time dance. Use a hostname, not a raw IP, and keep the URI stable: changing it later means rerunning consent.",
          'Set the refresh-token validity to the maximum (90 days). The portal warns you ten days before it lapses; openrow rotates it on every API call so a regularly-used connector never reaches that warning.',
          'Save the configuration.',
        ],
      },
      {
        title: '4. Verify against the sandbox',
        blurb:
          'Before requesting production access, prove the integration works against the sandbox. You can do every step here without bank approval.',
        steps: [
          "In the application's Sandbox area, generate test credentials. You'll see three values: API Key, Client ID, Client Secret. These are sandbox-only.",
          "Run the OAuth consent flow against the sandbox authorization endpoint shown in the portal, with response_type=code, scope=siblings.accounts, and your redirect URI. The portal displays test user credentials for the login screen.",
          "When the redirect lands at your URI it carries a code= query parameter. POST that code (with grant_type=authorization_code, your client_id, client_secret, and redirect_uri) to the sandbox token endpoint. The response includes the refresh_token you'll paste into openrow.",
          "In openrow open Settings → Connectors → Česká spořitelna. Paste Client ID, Client Secret, WEB-API key, Refresh token. Set Environment to sandbox. Click Test connection. A green tick means the credentials roundtrip works end to end.",
        ],
      },
      {
        title: '5. Enable 2FA and request production access',
        blurb:
          "Production credentials require two-factor on your developer account and a per-application approval from Česká spořitelna. The approval is a human review on the bank's side.",
        steps: [
          'Open your developer-account settings and enable two-factor authentication. The portal supports TOTP apps (Google Authenticator, 1Password, Bitwarden, Authy). Save the recovery key somewhere you trust; losing it locks you out if the device dies.',
          "Switch the application's environment tab to Production.",
          "Fill in the organisational details ČS requires: tax ID (DIČ for Czech entities), legal name, registered address, contact e-mail. These fields appear before the submission button is enabled.",
          'Open Bank Approval, review the summary, and submit. The bank reviews asynchronously. Expect a few business days; the portal e-mails you when the decision lands.',
        ],
      },
      {
        title: '6. Wire production into openrow',
        blurb:
          "Once the bank approves the application, the production credentials become available. The flow mirrors the sandbox dance, against the live gateway this time.",
        steps: [
          "Open the application in Production. The API Key, Client ID, and Client Secret now have production values. Reading the secret prompts a 2FA challenge.",
          "Run the OAuth consent flow again, this time against the production authorization endpoint. The login screen is the real Česká spořitelna George login. Sign in with your banking identity and approve the requested scope.",
          'Capture the production refresh_token the same way you did in sandbox: exchange the authorization code at the production token endpoint, copy refresh_token from the response.',
          'Update the openrow connector config: paste the production values, clear the Environment field (or set it to production), and click Test connection. From this point on openrow handles token refresh on its own.',
        ],
      },
    ],
    fields: [
      { label: 'Client ID', required: true, description: 'From the EDP application settings, production tab once approved.' },
      { label: 'Client Secret', required: true, secret: true, description: 'Paired with the Client ID. The portal asks for 2FA before revealing the production value.' },
      {
        label: 'WEB-API key',
        required: true,
        secret: true,
        description:
          'Sent as the WEB-API-key header on every request. In the sandbox it often equals the Client ID; in production it is a separate value.',
      },
      {
        label: 'Refresh token',
        required: true,
        secret: true,
        description: 'Captured once at the end of the OAuth consent flow with scope siblings.accounts. openrow rotates it on every refresh.',
      },
      {
        label: 'Environment',
        placeholder: 'production',
        description: "Set to 'sandbox' while you verify the integration; blank or 'production' for the live API.",
      },
    ],
    actions: [
      { name: 'List accounts', description: 'All authorised business accounts with IBAN, currency, product, and balance.' },
      {
        name: 'List transactions',
        description:
          'Transactions for one account between fromDate and toDate (inclusive). Newest first. Compact projection with amount, counterparty, VS/KS/SS symbols and description.',
      },
      { name: 'Get account', description: 'Fetch a single account by id.' },
    ],
    references: [
      { label: 'Erste Developer Portal', url: 'https://developers.erstegroup.com' },
      { label: 'George Developer API (overview)', url: 'https://developers.erstegroup.com/docs/apis/bank.csas/' },
    ],
    example: "Show me unmatched incoming payments on ČS from the last 7 days.",
  },
  {
    id: 'csob',
    name: 'ČSOB',
    category: 'banking',
    status: 'available',
    homepage: 'https://api.csob.cz',
    short: 'Read own-account transactions via the ČSOB API Portal (OAuth 2.0).',
    intro:
      'Read-only access to your ČSOB accounts via the official API Portal. Backed by OAuth 2.0 with a long-lived refresh token: paste once, working until you rotate.',
    steps: [
      'Register on the ČSOB API Portal at api.csob.cz and create a new Application.',
      'Note the Client ID and Client Secret. Add a redirect URI you control for the consent flow.',
      'Walk through the OAuth consent for the accounts scope. Capture the refresh_token in the final token response.',
      "In Settings → Connectors → ČSOB, paste Client ID, Client Secret and Refresh token. Leave API base URL and Environment blank for production.",
      'Click Test connection. A green tick means a successful token refresh.',
    ],
    fields: [
      { label: 'Client ID', required: true, description: 'From ČSOB API Portal → My Applications.' },
      { label: 'Client Secret', required: true, secret: true, description: 'Issued alongside the Client ID.' },
      {
        label: 'Refresh token',
        required: true,
        secret: true,
        description: 'Obtained once via the OAuth consent flow for the accounts scope.',
      },
      {
        label: 'API base URL',
        description:
          'Leave blank for production. Set manually if ČSOB assigned you a different host.',
      },
      {
        label: 'Environment',
        placeholder: 'production',
        description: "Use 'sandbox' to default to the sandbox host instead of production.",
      },
    ],
    actions: [
      { name: 'List accounts', description: 'Consented ČSOB accounts with IBAN, currency and balance.' },
      {
        name: 'List transactions',
        description:
          'Transactions for one account between dateFrom and dateTo (inclusive). Compact projection with amount, counterparty and Czech payment symbols (VS/KS/SS) extracted from remittance info.',
      },
    ],
    example: 'Pull yesterday\'s ČSOB transactions and try to match them against open invoices.',
  },
  {
    id: 'fio',
    name: 'Fio banka',
    category: 'banking',
    status: 'available',
    homepage: 'https://www.fio.cz',
    short: 'Read transactions and balance via a per-account API token.',
    intro:
      'Fio gives you a per-account API token with no OAuth. The simplest banking integration in the catalog: open Internet Banking, generate a read-only token, paste it here.',
    steps: [
      "Sign into Fio Internet Banking with the account you want to expose.",
      'Open Nastavení → API. Click Vytvořit token (Create token).',
      "Pick read-only scope. Optionally restrict the token's IP range to your server.",
      'Copy the token immediately — Fio shows it only once.',
      'In Settings → Connectors → Fio banka, paste the API token and click Test connection.',
    ],
    fields: [
      {
        label: 'API token',
        required: true,
        secret: true,
        description: 'Internetbanking → Nastavení → API → Vytvořit token. Pick read-only scope.',
      },
    ],
    actions: [
      {
        name: 'List transactions',
        description: 'Transactions in a date range (YYYY-MM-DD, inclusive). Newest first.',
      },
      {
        name: 'Account info',
        description: 'IBAN, currency, and closing balance for the configured account.',
      },
    ],
    example: 'List Fio transactions from the past 14 days and group them by counterparty.',
  },
  {
    id: 'revolut',
    name: 'Revolut Business',
    category: 'banking',
    status: 'available',
    homepage: 'https://business.revolut.com',
    short: 'Business banking — balances and transactions across currencies.',
    intro:
      "Revolut Business uses certificate-based OAuth: you upload an X.509 public key, Revolut hands you a Client ID, and you sign JWTs with the matching private key. The refresh token is valid for 90 days — openrow rotates it on every successful refresh.",
    steps: [
      'Generate a 2048-bit RSA key pair (openssl genrsa -out private.pem 2048 && openssl rsa -in private.pem -pubout -out public.pem).',
      'In Revolut Business → Settings → APIs, click Add API certificate. Upload public.pem. Pick a reasonable expiry.',
      'Note the Client ID Revolut shows after upload.',
      "Pick a JWT issuer — typically your domain (e.g. acme.com). It must match the iss claim on every token request.",
      'Run the one-time consent flow at business.revolut.com to authorise the app for your accounts.',
      'Capture the refresh_token from the consent callback. Treat it like a password.',
      'In Settings → Connectors → Revolut Business, paste Client ID, Issuer, Private key (full PEM including BEGIN/END), Refresh token. Leave Environment blank for production.',
      'Click Test connection to confirm the credentials roundtrip works.',
    ],
    fields: [
      { label: 'Client ID', required: true, description: 'From Revolut Business → Settings → APIs after uploading your public key.' },
      {
        label: 'Issuer',
        required: true,
        placeholder: 'your.domain.com',
        description: 'JWT issuer registered with your API client — typically your domain.',
      },
      {
        label: 'Private key (PEM)',
        required: true,
        secret: true,
        description: 'The full PEM including BEGIN/END lines. Must match the public key uploaded to Revolut.',
      },
      {
        label: 'Refresh token',
        required: true,
        secret: true,
        description: 'Obtained once via the consent flow. Valid for 90 days; openrow rotates it on use.',
      },
      {
        label: 'Environment',
        placeholder: 'production',
        description: "Use 'sandbox' for the test environment; leave blank for production.",
      },
    ],
    actions: [
      {
        name: 'List accounts',
        description: 'All business accounts with balance, currency, and state.',
      },
      {
        name: 'List transactions',
        description: 'Newest first. Filter by account, date range, counterparty, state.',
      },
      { name: 'Get account', description: 'Single account by id — balance, currency, state, IBAN/BIC.' },
    ],
    example: 'How much money do I have across all Revolut accounts in EUR equivalent?',
  },
  // --- Billing ---------------------------------------------------------
  {
    id: 'fakturoid',
    name: 'Fakturoid',
    category: 'billing',
    status: 'available',
    homepage: 'https://www.fakturoid.cz',
    short: 'Czech invoicing SaaS — sync invoices, clients and payments.',
    intro:
      "Fakturoid's v3 API uses OAuth 2.0 client credentials. Tokens are short-lived (~2h); openrow refreshes them transparently. Fakturoid also asks every integration to identify itself via User-Agent so they can reach you if something looks off.",
    steps: [
      'Open Fakturoid → Nastavení → Uživatelský účet → API.',
      'Click Vytvořit API klienta. Pick the Client credentials flow (no user consent step).',
      'Note the Client ID and Client Secret.',
      "Find your account slug — it's the subdomain in app.fakturoid.cz/<slug>.",
      'In Settings → Connectors → Fakturoid, paste slug, client_id, client_secret, and a contact e-mail.',
      'Click Test connection — it calls /account.json to confirm the credentials are good.',
    ],
    fields: [
      { label: 'Account slug', required: true, placeholder: 'your-company', description: 'The subdomain part of app.fakturoid.cz/<slug>.' },
      {
        label: 'OAuth client ID',
        required: true,
        description: "Create an API client under Nastavení → Uživatelský účet → API. Pick the Client credentials flow.",
      },
      { label: 'OAuth client secret', required: true, secret: true, description: 'Paired with the Client ID.' },
      {
        label: 'Contact e-mail',
        required: true,
        placeholder: 'you@example.com',
        description: 'Sent in the User-Agent header so Fakturoid can reach you if something goes wrong.',
      },
    ],
    actions: [
      {
        name: 'List invoices',
        description: 'Filter by status (open / paid / overdue / cancelled), invoice number substring, or issue date. Compact projection (id, number, status, total, remaining_amount, due_on).',
      },
      { name: 'Find invoice by number', description: 'Exact lookup. Returns null if there is no match.' },
      {
        name: 'Mark invoice paid',
        mutates: true,
        description: 'Idempotent — calling on an already-paid invoice is a no-op.',
      },
    ],
    example: 'Which Fakturoid invoices are overdue right now?',
  },
  {
    id: 'uol',
    name: 'ÚOL',
    category: 'billing',
    status: 'available',
    homepage: 'https://www.uol.cz',
    short: 'Účetnictví Online — read & create sales invoices, list receivables, see bank movements.',
    intro:
      'UOL (Účetnictví Online) is a Czech cloud accounting suite popular with small studios and consultancies. The REST API uses HTTP Basic — your user e-mail and a per-user API token.',
    steps: [
      "Sign into UOL and note your subdomain — the part before .ucetnictvi.uol.cz. That's your Customer ID.",
      'Open Settings → API tokens (the /api_tokens path).',
      'Create a token for the user whose permissions you want to inherit. Tick REST API permission.',
      'Copy the token — UOL only shows it once.',
      'In Settings → Connectors → ÚOL, paste Customer ID, user e-mail (HTTP Basic username), and API token.',
      "Use 'demo' as Environment to point at the shared sandbox; blank means production.",
    ],
    fields: [
      {
        label: 'Customer ID',
        required: true,
        placeholder: 'acme',
        description: "Subdomain assigned by UOL: the 'acme' in https://acme.ucetnictvi.uol.cz. Use 'test' for the demo host.",
      },
      { label: 'User e-mail', required: true, description: 'The UOL user whose API token this is. Sent as HTTP Basic username.' },
      { label: 'API token', required: true, secret: true, description: 'From UOL settings → /api_tokens. Requires REST API permission.' },
      {
        label: 'Environment',
        placeholder: 'production',
        description: "Use 'demo' to force the shared sandbox host; blank defaults to production.",
      },
    ],
    actions: [
      { name: 'List sales invoices', description: 'Vystavené faktury. Filter by VS, issue date, external_id, or type.' },
      { name: 'Get sales invoice', description: 'One invoice by public_id (e.g. 2020000003).' },
      { name: 'List purchase invoices', description: 'Přijaté faktury. Filter by seller, due/received date range, type.' },
      { name: 'Get purchase invoice', description: 'One přijatá faktura by public_id.' },
      { name: 'List receivables', description: 'Pohledávky. Filter by state and "overdue as of" date. Rate-limited to 10 req / 10 s.' },
      { name: 'List bank movements', description: 'Bank movements as tracked by UOL, paired with invoices where applicable.' },
      { name: 'List contacts', description: 'Counterparties. Filter by name, IČO, DIČ, or free text.' },
      {
        name: 'Create sales invoice',
        mutates: true,
        description: "Minimum body is {buyer_id, items:[{product_id, unit_price, quantity}]}. Set type='proforma' for zálohová.",
      },
    ],
    example: "Issue a UOL invoice from the latest row of draft_invoices.",
  },
  // --- ERP -------------------------------------------------------------
  {
    id: 'flexi',
    name: 'ABRA Flexi',
    category: 'erp',
    status: 'available',
    homepage: 'https://www.abra.eu/flexi/',
    short: 'Czech SMB ERP — AI-native binding maps evidences to canonical roles.',
    intro:
      "Flexi exposes hundreds of Czech-named evidences (faktury-vydane, adresar, sklad-karty…). On first connect openrow scans the evidence list and proposes a mapping to canonical roles (customer, product, invoice_outgoing, …). You confirm the mapping, then the agent queries by role instead of memorising Flexi-specific names.",
    steps: [
      'Pick the Flexi server you connect to — your hosted instance, on-prem, or the public demo at https://demo.flexibee.eu.',
      'Note your company token: the segment after /c/ in the Flexi URL (e.g. demo_s_r_o_).',
      'Create or pick a user account with API access. Note its username and password.',
      'In Settings → Connectors → ABRA Flexi, paste the Server URL (without /c/<company>), company token, username, password.',
      'Click Test connection. On first success openrow runs evidence discovery — review the proposed role mapping and activate it.',
    ],
    fields: [
      {
        label: 'Server URL',
        required: true,
        placeholder: 'https://demo.flexibee.eu',
        description: 'Root URL of the Flexi server (no trailing /c/<company>).',
      },
      {
        label: 'Company token',
        required: true,
        placeholder: 'demo_s_r_o_',
        description: 'Company segment from your Flexi URL — the part after /c/.',
      },
      { label: 'Username', required: true, description: 'A Flexi user with API access.' },
      { label: 'Password', required: true, secret: true, description: 'The matching password.' },
    ],
    actions: [
      {
        name: 'Query Flexi',
        description: 'Read rows from a Flexi evidence by canonical role (customer, product, invoice_outgoing…). Columns are mapped back from Czech.',
      },
    ],
    example: "List the last 20 invoice_outgoing rows from Flexi.",
  },
  // --- Payments --------------------------------------------------------
  {
    id: 'stripe',
    name: 'Stripe',
    category: 'payments',
    status: 'coming_soon',
    homepage: 'https://stripe.com',
    short: 'Card payments and subscriptions. Webhook verifier live; actions in progress.',
    intro:
      "Stripe's webhook signature verifier is wired up — a flow with a webhook trigger can authenticate Stripe events today. Read/sync actions (charges, payouts, fees) are still being built; the connector card shows as Coming Soon until they ship.",
    steps: [
      'Create a Stripe account and an API restricted key with read access on charges, payouts, customers.',
      "(Optional) Create a webhook endpoint in Stripe pointed at your openrow webhook URL. Copy the whsec_… signing secret.",
      'In Settings → Connectors → Stripe, paste the secret_key and (if applicable) the webhook signing secret.',
    ],
    fields: [
      {
        label: 'Secret key',
        required: true,
        secret: true,
        placeholder: 'sk_live_…',
        description: 'Restricted keys with read access on charges, payouts and customers are recommended.',
      },
      {
        label: 'Webhook signing secret',
        secret: true,
        placeholder: 'whsec_…',
        description: 'Optional. Enables near-real-time sync via webhook.',
      },
    ],
    actions: [],
    webhook:
      'Stripe webhook signatures are verified using the Stripe-Signature header (t=… , v1=…) with HMAC-SHA256 over `<timestamp>.<body>` and a 5-minute replay tolerance, matching the Stripe SDK default.',
  },
  // --- Chat ------------------------------------------------------------
  {
    id: 'slack',
    name: 'Slack',
    category: 'chat',
    status: 'available',
    homepage: 'https://slack.com',
    short: 'Post messages to Slack channels from flows and the agent.',
    intro:
      "Slack as the output channel for every alert, digest, and approval reminder. Set a default channel and the agent can fire off a message in any flow without you re-typing the channel name.",
    steps: [
      'Open api.slack.com/apps and click Create New App → From scratch. Pick a workspace.',
      'Under OAuth & Permissions, add the chat:write scope (and chat:write.public if you want to post to channels without inviting the bot).',
      'Click Install to Workspace. Note the Bot User OAuth Token — starts with xoxb-.',
      'Invite the bot to every channel you intend to post in (or grant chat:write.public to skip this).',
      'In Settings → Connectors → Slack, paste the Bot token. Optionally set a default channel (e.g. #faktury).',
    ],
    fields: [
      {
        label: 'Bot token',
        required: true,
        secret: true,
        placeholder: 'xoxb-…',
        description: 'From a Slack app with at least chat:write. Invite the bot to any channel you want to post in.',
      },
      {
        label: 'Default channel',
        placeholder: '#general',
        description: "Used when a flow or action doesn't specify one.",
      },
    ],
    actions: [
      {
        name: 'Post message',
        mutates: true,
        description: 'Send a message to a Slack channel. Supports threading via thread_ts.',
      },
    ],
    example: 'Ping #faktury when an invoice goes overdue, and again two reminders later if still unpaid.',
  },
  {
    id: 'discord',
    name: 'Discord',
    category: 'chat',
    status: 'available',
    homepage: 'https://discord.com',
    short: 'Post messages to a Discord channel via an incoming webhook.',
    intro:
      "Discord webhooks are the simplest path: per-channel, no app, no bot user. Generate a URL, paste it, send.",
    steps: [
      'Open the target Discord server. Server Settings → Integrations → Webhooks → New Webhook.',
      'Pick the channel and (optionally) a name and avatar for the webhook.',
      'Click Copy Webhook URL.',
      'In Settings → Connectors → Discord, paste the URL and click Test connection.',
    ],
    fields: [
      {
        label: 'Webhook URL',
        required: true,
        secret: true,
        placeholder: 'https://discord.com/api/webhooks/…',
        description: 'Server Settings → Integrations → Webhooks → New Webhook → Copy URL.',
      },
    ],
    actions: [
      {
        name: 'Post message',
        mutates: true,
        description: 'Send a message to the configured Discord channel.',
      },
    ],
    example: 'Send a Discord ping whenever a new contact is created.',
  },
  // --- Email -----------------------------------------------------------
  {
    id: 'resend',
    name: 'Resend',
    category: 'email',
    status: 'available',
    homepage: 'https://resend.com',
    short: 'Transactional email — verify a domain, send from any address on it.',
    intro:
      "Resend is the email provider for invoice deliveries, overdue reminders, and password resets. Quick to set up: verify your domain, paste an API key, set a default From address.",
    steps: [
      'Create a Resend account and add your sending domain.',
      'Add the DNS records Resend shows (SPF, DKIM). Wait for the green tick.',
      'Under API Keys, create a key with Full Access (or Sending Access).',
      "In Settings → Connectors → Resend, paste the API key. Set a default From (e.g. invoices@yourdomain.com) so flows don't need to repeat it.",
      'Click Test connection — it calls /domains to confirm the key works.',
    ],
    fields: [
      { label: 'API key', required: true, secret: true, placeholder: 're_…', description: 'A Resend API key with sending access.' },
      {
        label: 'Default From',
        placeholder: 'you@yourdomain.com',
        description: "Used when a send_email call doesn't set 'from'.",
      },
    ],
    actions: [
      {
        name: 'Send email',
        mutates: true,
        description: 'Send a transactional email. At least one of html or text must be set.',
      },
    ],
    example: 'Email the rendered PDF of invoice 2026001 to its client.',
  },
  // --- Docs ------------------------------------------------------------
  {
    id: 'notion',
    name: 'Notion',
    category: 'docs',
    status: 'available',
    homepage: 'https://notion.so',
    short: 'Create pages and query Notion databases.',
    intro:
      "Mirror finance, client, or project records into Notion so the rest of the team can see them without an openrow login. Internal Notion integrations only — no public OAuth dance.",
    steps: [
      'Go to https://www.notion.so/profile/integrations and click New integration.',
      'Type: Internal. Pick the workspace, give it a name (e.g. openrow). Save.',
      'Copy the Internal Integration Token — starts with secret_ or ntn_.',
      'Open every Notion page or database you want openrow to access. Click ⋯ → Connections → add your integration. Children inherit access from their parents.',
      'In Settings → Connectors → Notion, paste the token. Test connection lists users to confirm the token is valid.',
    ],
    fields: [
      {
        label: 'Integration token',
        required: true,
        secret: true,
        placeholder: 'secret_… or ntn_…',
        description: 'Create an internal integration at notion.so/profile/integrations, then share each target page or database with it.',
      },
    ],
    actions: [
      {
        name: 'Create page',
        mutates: true,
        description: "Create a Notion page. 'parent' is either { database_id } or { page_id }.",
      },
      {
        name: 'Query database',
        description: "Query a Notion database. 'filter' and 'sorts' are Notion's native structures.",
      },
    ],
    example: "Mirror every new client into the Notion clients database.",
  },
  // --- Dev -------------------------------------------------------------
  {
    id: 'github',
    name: 'GitHub',
    category: 'dev',
    status: 'available',
    homepage: 'https://github.com',
    short: 'Create and comment on issues; receive webhooks.',
    intro:
      "GitHub is one of two ways flows can react to outside events: webhooks (push, issue events) or polling. The connector also lets the agent open or comment on issues — useful for funnelling bug reports into Linear-style triage from your own data.",
    steps: [
      'Open github.com → Settings → Developer settings → Personal access tokens → Fine-grained tokens.',
      'Generate a new token. Scope it to the repos you want to touch. Permissions: Issues → Read and write.',
      'Copy the token (starts with github_pat_ or ghp_).',
      'Optionally set a default owner and repo so actions can omit them.',
      "(Optional) If a flow is webhook-triggered, generate a strong random Webhook secret and paste it both in GitHub's webhook settings and here.",
      'In Settings → Connectors → GitHub, paste the token. Test connection calls /user to confirm the token works.',
    ],
    fields: [
      {
        label: 'Personal access token',
        required: true,
        secret: true,
        placeholder: 'ghp_… or github_pat_…',
        description: 'Fine-grained PAT scoped to the repos you want to touch, with Issues: Read & write.',
      },
      { label: 'Default owner', placeholder: 'my-org', description: 'Used when an action omits owner/repo.' },
      { label: 'Default repo', placeholder: 'my-repo', description: 'Used when an action omits owner/repo.' },
      {
        label: 'Webhook secret',
        secret: true,
        description: 'Only required if flows are triggered by GitHub webhooks. Verified via HMAC-SHA256.',
      },
    ],
    actions: [
      { name: 'Create issue', mutates: true, description: 'Open a new issue on a repo.' },
      { name: 'Comment on issue', mutates: true, description: 'Add a comment to an existing issue or pull request.' },
      { name: 'Add labels', mutates: true, description: 'Add labels (additive; does not replace).' },
    ],
    webhook:
      'GitHub webhooks are verified via the X-Hub-Signature-256 header using HMAC-SHA256 of the request body against your webhook secret.',
    example: 'When a bug row is created with priority=high, open a GitHub issue.',
  },
  {
    id: 'linear',
    name: 'Linear',
    category: 'dev',
    status: 'available',
    homepage: 'https://linear.app',
    short: 'Create and update Linear issues.',
    intro:
      "Linear is the inverse of the GitHub connector: rather than reading issue events, openrow pushes new issues into your tracker. Useful when your data raises questions a human should answer.",
    steps: [
      'Linear → Settings → API → Personal API keys → Create key.',
      "Name it (e.g. 'openrow') and copy the key — starts with lin_api_.",
      "Note the team id of the team that issues should default to. You can find it in any issue's URL.",
      'In Settings → Connectors → Linear, paste the API key and (optionally) default team id.',
    ],
    fields: [
      {
        label: 'API key',
        required: true,
        secret: true,
        placeholder: 'lin_api_…',
        description: 'Linear → Settings → API → Personal API keys.',
      },
      {
        label: 'Default team ID',
        description: "Used when create_issue doesn't specify a team.",
      },
    ],
    actions: [
      { name: 'Create issue', mutates: true, description: "Create an issue in a team. Returns id, identifier (e.g. ENG-42) and URL." },
      { name: 'Update issue state', mutates: true, description: 'Move an issue to a different workflow state.' },
    ],
    example: 'Whenever a flow run fails twice in a row, file a Linear issue with the run id.',
  },
  // --- Registry --------------------------------------------------------
  {
    id: 'ares',
    name: 'ARES',
    category: 'registry',
    status: 'available',
    homepage: 'https://ares.gov.cz',
    short: 'Czech company registry — look up by IČO or by name.',
    intro:
      "ARES is the official Czech business registry. No credentials, no rate limit headaches. Useful for autofilling clients (address, DIČ, legal form) when you only know the IČO.",
    steps: ['No setup. The connector is enabled in every workspace.'],
    fields: [],
    actions: [
      { name: 'Lookup by IČO', description: 'Return the company registered under this IČO (8 digits), or null.' },
      { name: 'Search by name', description: 'Full or partial name match. Up to limit compact matches.' },
    ],
    example: "Look up IČO 26168685 and add the company to clients.",
  },
  {
    id: 'vies',
    name: 'VIES (EU VAT)',
    category: 'registry',
    status: 'available',
    homepage: 'https://ec.europa.eu/taxation_customs/vies/',
    short: 'Validate EU VAT numbers against the European Commission database.',
    intro:
      "VIES is the European Commission's VAT validation service. The agent uses it to verify clients' DIČ before issuing cross-border invoices.",
    steps: ['No setup. The connector is enabled in every workspace.'],
    fields: [],
    actions: [
      {
        name: 'Validate VAT number',
        description: "Accepts a combined VAT (CZ12345678) or country_code + vat_number. Returns valid yes/no plus name and address when registered.",
      },
    ],
    example: 'Validate DIČ DE123456789 before issuing the invoice.',
  },
  // --- Reference -------------------------------------------------------
  {
    id: 'cnb',
    name: 'ČNB FX rates',
    category: 'reference',
    status: 'available',
    homepage: 'https://www.cnb.cz',
    short: 'Daily exchange rates published by the Czech National Bank.',
    intro:
      "Authoritative CZK rates for every currency the ČNB publishes, dating back decades. The agent uses it to convert foreign currency invoices and bank movements into CZK at the correct daily rate.",
    steps: ['No setup. The connector is enabled in every workspace.'],
    fields: [],
    actions: [
      { name: 'Get rate', description: 'CZK exchange rate for a currency code on a given date (default: today).' },
      { name: 'List rates', description: 'All CZK rates for a given date.' },
    ],
    example: "What was the EUR-to-CZK rate on 2026-01-15?",
  },
]
