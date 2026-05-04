# TypeScript + JavaScript

Use this skill for TypeScript, JavaScript, Node.js, React, Next.js, Vite, frontend apps, backend JS services, CLI scripts, and package/library work. Keep the output practical, repo-aware, and minimal-diff. Do not use this skill just because a web UI exists; use the Web skill only when the task is visual/design quality.

---

## Routing Rules

Use when the repo or task includes any of:

- `package.json`, `tsconfig.json`, `jsconfig.json`, `vite.config.*`, `next.config.*`, `astro.config.*`, `svelte.config.*`, `nuxt.config.*`
- `.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs` files
- React, Next.js, Vue, Svelte, Astro, Node.js, Express, Fastify, NestJS, Hono, tRPC, Prisma, Drizzle, Vite, Vitest, Jest, Playwright, Cypress

Do not use when the task is purely design guidance with no JS/TS implementation, or when the changed files are clearly in another language.

---

## Operating Standard

Before editing, infer the stack from actual files, not assumptions:

- Package manager: respect the existing lockfile: `pnpm-lock.yaml`, `yarn.lock`, `package-lock.json`, `bun.lockb` / `bun.lock`.
- Module system: check `package.json` `type`, file extensions, `tsconfig.module`, existing imports.
- Framework conventions: follow the app’s routing, data fetching, state management, styling, and test patterns.
- Existing style: mirror formatting, naming, folder structure, error handling, and component patterns.

Prefer small, correct changes over broad rewrites. Touch only the files required by the task unless a nearby fix is necessary for correctness.

---

## 80/20 Rules

### 1. Make the type system carry the design

TypeScript should prevent invalid states instead of documenting them.

Prefer:

```ts
// Good: state is explicit and exhaustive
type LoadState<T> =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "success"; data: T }
  | { status: "error"; error: string };
```

Avoid vague bags of optionals:

```ts
// Bad: many impossible states are representable
type LoadState<T> = {
  loading?: boolean;
  data?: T;
  error?: string;
};
```

Use `unknown` at unsafe boundaries, then narrow or validate. Avoid `any` unless the surrounding code already uses it and the safer conversion would be disproportionate.

```ts
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
```

Use `satisfies` for config objects where literal precision matters:

```ts
const routes = {
  home: "/",
  settings: "/settings",
} as const satisfies Record<string, `/${string}`>;
```

### 2. Treat external input as hostile

Validate boundaries: request bodies, query params, localStorage, env vars, webhooks, JSON files, AI/tool outputs, database rows if schema drift is possible.

With Zod or equivalent already in the repo:

```ts
import { z } from "zod";

const CreateUserSchema = z.object({
  email: z.string().email(),
  name: z.string().trim().min(1).max(80),
});

type CreateUserInput = z.infer<typeof CreateUserSchema>;

const input: CreateUserInput = CreateUserSchema.parse(await request.json());
```

If no validator exists, use lightweight guards rather than adding a dependency without reason.

### 3. React: keep render pure and state minimal

Do not put derived values in state. Compute them from props/state.

```tsx
const filteredItems = useMemo(
  () => items.filter((item) => item.name.includes(query)),
  [items, query],
);
```

Use stable keys from data, not indexes, unless the list is static and never reordered.

```tsx
{items.map((item) => (
  <ItemRow key={item.id} item={item} />
))}
```

Never call async work directly in render. Put effects in `useEffect`, event handlers, framework loaders/actions, server components, or query libraries.

```tsx
useEffect(() => {
  const controller = new AbortController();

  void loadItems({ signal: controller.signal }).then(setItems).catch((error) => {
    if (error.name !== "AbortError") setError(String(error));
  });

  return () => controller.abort();
}, []);
```

Do not silence hook dependency warnings by deleting dependencies. Fix stale closures with functional updates, stable callbacks, or moving logic out of the effect.

```tsx
setCount((current) => current + 1);
```

### 4. Next.js: respect server/client boundaries

Default to Server Components when possible. Add `"use client"` only for components that need browser APIs, local state, effects, refs, or event handlers.

Do not import server-only modules into client components: filesystem, database clients, secrets, admin SDKs, private env vars.

```tsx
// Server component: can fetch from DB or private API
export default async function Page() {
  const projects = await getProjects();
  return <ProjectList projects={projects} />;
}
```

```tsx
"use client";

export function ProjectList({ projects }: { projects: Project[] }) {
  const [query, setQuery] = useState("");
  // browser interactivity here
}
```

For route handlers, validate input, return correct status codes, and avoid leaking raw internal errors.

```ts
export async function POST(request: Request) {
  try {
    const input = CreateUserSchema.parse(await request.json());
    const user = await createUser(input);
    return Response.json({ user }, { status: 201 });
  } catch (error) {
    return Response.json({ error: "Invalid request" }, { status: 400 });
  }
}
```

### 5. Node/API code: handle async failures deliberately

Always await promises that must complete. Do not rely on floating promises unless intentionally fire-and-forget, and then make error handling explicit.

```ts
void analytics.track(event).catch((error) => {
  logger.warn({ error }, "analytics event failed");
});
```

For Express-style handlers, use the project’s existing async wrapper if present. Otherwise use `next(error)` or local `try/catch`.

```ts
app.post("/users", async (req, res, next) => {
  try {
    const input = CreateUserSchema.parse(req.body);
    const user = await users.create(input);
    res.status(201).json({ user });
  } catch (error) {
    next(error);
  }
});
```

Never trust `req.body`, `req.query`, headers, cookies, filenames, or user-provided URLs.

### 6. Keep package changes conservative

Before adding a dependency, check whether the repo already has a library that solves the problem. Avoid adding large libraries for small utilities.

When adding scripts or packages:

- preserve the package manager
- preserve dependency style: `dependencies` vs `devDependencies`
- avoid major upgrades unless requested
- check ESM/CJS compatibility
- check browser/server compatibility

Common package-manager commands:

```bash
# Use the one matching the lockfile
pnpm install
npm install
yarn install
bun install
```

```bash
pnpm add <pkg>
npm install <pkg>
yarn add <pkg>
bun add <pkg>
```

### 7. Prefer repo-native verification

Run the narrowest meaningful checks available:

```bash
pnpm typecheck
pnpm lint
pnpm test
pnpm test -- --runInBand
pnpm vitest run
pnpm playwright test
pnpm build
```

If scripts are unknown, inspect `package.json`. Do not invent commands.

When a check fails, distinguish:

- caused by your change
- pre-existing failure
- environment/dependency issue
- missing secrets/services

### 8. Frontend behavior must include real states

For UI work, implement loading, empty, error, disabled, and success states when relevant. A component that only looks good in the happy path is incomplete.

```tsx
if (state.status === "loading") return <Spinner label="Loading projects" />;
if (state.status === "error") return <ErrorState message={state.error} />;
if (state.status === "success" && state.data.length === 0) return <EmptyState />;
```

Do not make buttons submit twice. Disable or guard while pending.

```tsx
<button disabled={isPending} aria-busy={isPending}>
  {isPending ? "Saving..." : "Save"}
</button>
```

### 9. Performance: fix the obvious, avoid fake optimization

Do optimize:

- repeated expensive work inside render
- unstable props causing large child rerenders
- excessive bundle imports
- network waterfalls
- unbounded list rendering
- leaking intervals/listeners/subscriptions

Do not scatter `useMemo`/`useCallback` everywhere by default. Use them when identity or computation cost matters.

Cleanup side effects:

```tsx
useEffect(() => {
  const id = window.setInterval(refresh, 30_000);
  return () => window.clearInterval(id);
}, [refresh]);
```

### 10. Accessibility is part of correctness

Use native elements first. Buttons for actions, anchors for navigation, labels for inputs.

```tsx
<label htmlFor="email">Email</label>
<input id="email" name="email" type="email" autoComplete="email" />
```

Interactive custom components need keyboard support, focus states, and ARIA only where it improves semantics. Do not paste ARIA attributes randomly.

---

## Common AI Mistakes To Avoid

- Rewriting whole files when a surgical patch is enough.
- Adding `any`, `// @ts-ignore`, or disabling ESLint to make errors disappear.
- Changing package manager or lockfile style.
- Importing browser-only code into server code, or server-only code into client bundles.
- Adding `"use client"` too high in a Next.js tree.
- Creating duplicate state for values that can be derived.
- Using array index as key on dynamic lists.
- Forgetting loading/error/empty states.
- Swallowing errors with empty `catch` blocks.
- Returning HTTP 200 for failed API operations.
- Logging secrets, tokens, cookies, auth headers, or full request bodies.
- Trusting `process.env.X!` without checking critical config at startup.
- Creating memory leaks with intervals, subscriptions, event listeners, sockets, or observers.
- Adding dependencies for tiny helpers.
- Assuming ESM/CJS interop works without checking the project.
- Breaking public APIs or prop names without updating all call sites.
- Making visual changes that ignore the existing design system.

---

## High-Value Snippets

### Exhaustive switch

```ts
function assertNever(value: never): never {
  throw new Error(`Unexpected value: ${String(value)}`);
}

switch (state.status) {
  case "idle":
    return null;
  case "loading":
    return <Spinner />;
  case "success":
    return <List items={state.data} />;
  case "error":
    return <ErrorState message={state.error} />;
  default:
    return assertNever(state);
}
```

### Safe env helper

```ts
function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`Missing required environment variable: ${name}`);
  return value;
}

export const env = {
  databaseUrl: requiredEnv("DATABASE_URL"),
};
```

### Typed async result for expected failures

```ts
type Result<T, E = string> =
  | { ok: true; value: T }
  | { ok: false; error: E };

async function getUser(id: string): Promise<Result<User>> {
  const user = await db.user.findUnique({ where: { id } });
  if (!user) return { ok: false, error: "User not found" };
  return { ok: true, value: user };
}
```

### Minimal fetch wrapper

```ts
async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);

  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
  }

  return (await response.json()) as T;
}
```

Use a schema validator at the call site when the response is not trusted.

---

## Output Style

When completing JS/TS work, report:

- files changed
- core behavior changed
- checks run and result
- any important risk or follow-up

Keep summaries concrete. Avoid generic “improved code quality” claims unless you state exactly what improved.
