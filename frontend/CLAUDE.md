[Root](../CLAUDE.md) > **frontend**

# Frontend Module (`frontend/`)

## Module Responsibilities

The frontend is a React 19 single-page application providing the web management dashboard for AxonHub. It covers channel management, API key management, model management, request tracing, user/role management, system settings, cost tracking, and a built-in AI playground.

## Entry and Startup

- **`src/main.tsx`** -- Application entry point, React root render.
- **`src/app/`** -- Application shell and layout.
- **`src/routes/`** -- File-based routing via TanStack Router.
- **`vite.config.ts`** -- Vite build configuration with backend proxy.
- Dev server: `pnpm dev` (port 5173, proxies to backend 8090).
- Production build: `pnpm build` (output to `frontend/dist/`, then copied to `internal/server/static/dist/`).

## External Interfaces

### Routes (`src/routes/`)

**Authentication:**
- `/sign-in`, `/sign-up`, `/forgot-password`, `/initialization`

**Authenticated pages (`/_authenticated/`):**
- `/` -- Dashboard (daily requests, channel performance, model performance)
- `/channels` -- Channel management
- `/models` -- Model management
- `/api-keys` -- API key management
- `/users` -- User management
- `/roles` -- Role management
- `/projects` -- Project management
- `/system` -- System settings
- `/data-storages` -- Data storage configurations
- `/chats` -- AI chat playground
- `/help-center` -- FAQ and help
- `/settings/*` -- User profile, appearance, display, notifications

**Project-scoped pages (`/_authenticated/project/`):**
- `/project/requests`, `/project/requests/$requestId`
- `/project/traces`, `/project/traces/$traceId`
- `/project/threads`, `/project/threads/$threadId`
- `/project/api-keys`, `/project/roles`, `/project/users`
- `/project/prompts`, `/project/playground`

### GraphQL Communication (`src/gql/`)

- `graphql.ts` -- GraphQL client setup
- Feature-specific queries/mutations in each `features/*/data/*.ts`

## Key Subpackages

### `src/features/` -- Feature Modules

| Feature            | Directory                | Description                          |
|-------------------|--------------------------|--------------------------------------|
| Auth              | `features/auth/`         | Sign-in, sign-up, initialization     |
| Channels          | `features/channels/`     | Channel CRUD, bulk ops, templates    |
| API Keys          | `features/apikeys/`      | API key management                   |
| Models            | `features/models/`       | Model configuration                  |
| Requests          | `features/requests/`     | Request logs and detail view         |
| Traces            | `features/traces/`       | Distributed trace viewer             |
| Threads           | `features/threads/`      | Conversation threading               |
| Users             | `features/users/`        | User management                      |
| Roles             | `features/roles/`        | Role management                      |
| Projects          | `features/projects/`     | Project management                   |
| Project Users     | `features/proejct-users/`| Project-scoped user management       |
| Project Roles     | `features/project-roles/`| Project-scoped role management       |
| Dashboard         | `features/dashboard/`    | Analytics and performance charts     |
| System            | `features/system/`       | System settings                      |
| Prompts           | `features/prompts/`      | Prompt template management           |
| Chats             | `features/chats/`        | AI chat playground                   |
| Data Storages     | `features/data-storages/`| External storage configuration       |
| Onboarding        | `features/onboarding/`   | First-run onboarding                 |

### `src/components/` -- Shared Components

- `ui/` -- Shadcn/Radix UI primitives (buttons, dialogs, forms, tables, etc.)
- `ai-elements/` -- AI-specific UI components (messages, reasoning, tools, etc.)
- `layout/` -- App layout (sidebar, header, nav)
- `date-range-picker/` -- Date/time range picker
- Auth guards, permission guards, project guards

### `src/stores/` -- State Management (Zustand)

- `authStore.ts` -- Authentication state
- `projectStore.ts` -- Active project state

### `src/hooks/` -- Custom Hooks

- `use-debounce.ts`, `use-copy-to-clipboard.ts`, `use-pagination-search.ts`, `use-error-handler.ts`, `use-version-check.ts`, etc.

### `src/locales/` -- Internationalization

- `en.json` -- English translations
- `zh.json` -- Chinese translations

## Key Dependencies and Configuration

- **React 19** with TypeScript 5.8
- **Vite 7** with SWC plugin
- **TanStack Router** (file-based routing)
- **TanStack Query** (data fetching/caching)
- **TanStack Table** (data tables)
- **Shadcn/ui + Radix UI** (component library)
- **Tailwind CSS 4** (styling)
- **Zustand** (state management)
- **Zod 4** (schema validation)
- **i18next** (internationalization)
- **Recharts** (charts)
- **AI SDK** (`ai` package for chat playground)
- **pnpm** (package manager, required)

## Testing and Quality

- **E2E tests**: Playwright in `frontend/tests/*.spec.ts` (15 test files).
- **Test config**: `frontend/playwright.config.ts`
- **Linting**: ESLint + typescript-eslint (`pnpm lint`)
- **Formatting**: Prettier (`pnpm format`)
- **Unused deps**: Knip (`pnpm knip`)
- **Lint-staged**: Husky pre-commit hooks
- Run: `cd frontend && pnpm test:e2e`

## Related File List

- `frontend/package.json` -- Dependencies and scripts
- `frontend/vite.config.ts` -- Build config
- `frontend/playwright.config.ts` -- Test config
- `frontend/src/main.tsx` -- App entry
- `frontend/src/routes/__root.tsx` -- Root route
- `frontend/src/locales/en.json` -- English i18n
- `frontend/src/locales/zh.json` -- Chinese i18n

## Changelog

- **2026-02-21**: Initial module documentation created during architecture scan.
