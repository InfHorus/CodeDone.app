# Go

Use this skill when the task touches Go source, modules, tests, HTTP services, CLIs, goroutines, channels, context cancellation, or Go build/runtime errors.

Go rewards small, boring, explicit code. Keep changes idiomatic and minimal.

## Activation

Use for:

- `.go`, `go.mod`, `go.sum`, `go.work`, Go tests, Go CLI/server code, gRPC, Gin/Fiber/Chi/Echo, Cobra, database/sql, goroutine/channel bugs, race conditions, build tags.

Do not use for:

- JavaScript “go to” wording, or non-Go projects with unrelated Docker tooling.

## Core Rule

Prefer clear control flow, explicit errors, small interfaces near consumers, and standard library solutions unless the project already uses a framework.

## 80/20 Go Workflow

1. Read `go.mod` for module path and Go version.
2. Follow existing package layout; do not invent architecture.
3. Run `gofmt`/`go test` mentally or actually when possible.
4. Return errors with context; do not panic for normal failures.
5. Pass `context.Context` through request-scoped operations.
6. Keep goroutine lifetimes cancellable.
7. Avoid global mutable state unless existing design requires it.
8. Add dependencies only when the standard library or existing deps are not enough.

## Formatting and Build

Always assume Go code must be `gofmt` formatted.

Commands:

```bash
gofmt -w ./...
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Do not reorder imports manually in weird ways; `gofmt`/`goimports` should handle it.

## Project Layout

Do not force a layout. Common patterns:

```txt
cmd/app/main.go
internal/service/...
internal/http/...
pkg/...              # only if intended as public library API
```

Use `internal/` for code not meant to be imported externally. Avoid creating `pkg/` unless the project already uses it or the API is intentionally public.

## Error Handling

Do not ignore errors.

```go
if err != nil {
    return fmt.Errorf("load config: %w", err)
}
```

Avoid:

```go
result, _ := doThing()
```

Use sentinel errors or typed errors only when callers need to branch:

```go
if errors.Is(err, sql.ErrNoRows) { ... }
```

Do not use panic for expected runtime errors like bad input, missing rows, failed HTTP calls, or validation failures.

## Context

Pass context into I/O, database, HTTP, and long-running work.

```go
func (s *Service) GetUser(ctx context.Context, id int64) (*User, error) {
    return s.repo.GetUser(ctx, id)
}
```

For handlers:

```go
ctx := r.Context()
```

Do not store context in structs for request-scoped operations.

## HTTP Services

Use the existing router/framework. With standard library:

```go
func getUser(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(user); err != nil {
        http.Error(w, "encode response", http.StatusInternalServerError)
        return
    }
}
```

Rules:

- Validate request method/path/input.
- Set status codes intentionally.
- Do not leak internal errors to clients.
- Bound request body sizes for untrusted input.
- Use server timeouts in production servers.

Server pattern:

```go
srv := &http.Server{
    Addr:              ":8080",
    Handler:           mux,
    ReadHeaderTimeout: 5 * time.Second,
}
```

## JSON

Define request/response structs explicitly:

```go
type CreateUserRequest struct {
    Email string `json:"email"`
    Name  string `json:"name"`
}
```

Avoid returning internal database structs if they contain private fields or persistence-specific shape.

For strict API input, consider disallowing unknown fields:

```go
dec := json.NewDecoder(r.Body)
dec.DisallowUnknownFields()
```

## Database

With `database/sql`, always check errors and close rows.

```go
rows, err := db.QueryContext(ctx, query, tenantID)
if err != nil {
    return nil, fmt.Errorf("query users: %w", err)
}
defer rows.Close()

for rows.Next() { ... }
if err := rows.Err(); err != nil { ... }
```

Use parameterized queries. Placeholder syntax depends on driver:

- PostgreSQL: `$1`, `$2`
- MySQL/SQLite: `?`

Transactions:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()

// use tx, not db

return tx.Commit()
```

## Goroutines and Channels

Every goroutine should have a clear lifetime and cancellation path.

Good pattern:

```go
go func() {
    defer wg.Done()
    for {
        select {
        case <-ctx.Done():
            return
        case job := <-jobs:
            process(job)
        }
    }
}()
```

Avoid goroutine leaks:

- Blocking forever on sends/receives.
- No context cancellation.
- Starting goroutines in request handlers without lifecycle management.
- Writing to closed channels.

Use `go test -race` when touching concurrent code.

## Interfaces

Define small interfaces where consumed, not huge interfaces where implemented.

```go
type UserStore interface {
    GetUser(ctx context.Context, id int64) (*User, error)
}
```

Do not create abstract factories/repositories unless they simplify testing or follow existing architecture.

## Packages and Visibility

Names starting with uppercase are exported. Do not export unless needed by another package or public API.

Keep package names short and lower-case:

```go
package billing
```

Avoid package names like `utils` when a domain name is clearer.

## Testing

Use table-driven tests for variant-heavy logic:

```go
func TestNormalizeEmail(t *testing.T) {
    tests := []struct {
        name string
        in   string
        want string
    }{
        {"lowercase", "A@B.COM", "a@b.com"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := NormalizeEmail(tt.in)
            if got != tt.want {
                t.Fatalf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

Prefer `httptest` for handlers and temporary directories via `t.TempDir()`.

## CLI Tools

For simple CLIs, standard `flag` is often enough. Use Cobra only if the project already uses it or needs subcommands.

Read env vars explicitly and validate required values:

```go
addr := os.Getenv("ADDR")
if addr == "" {
    addr = ":8080"
}
```

## Common AI Mistakes

Avoid these:

- Ignoring errors with `_`.
- Creating unnecessary interfaces for every struct.
- Using global variables for request state.
- Forgetting `defer rows.Close()` or `rows.Err()`.
- Using `db` inside a transaction instead of `tx`.
- Starting goroutines without cancellation.
- Writing unformatted Go.
- Adding heavy dependencies for trivial standard-library tasks.
- Exporting everything by default.
- Misusing channels where a mutex or simple function call is clearer.
- Returning raw internal errors to HTTP clients.
- Reading unbounded request bodies.

## Verification

Run the narrowest useful checks:

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

For module/dependency changes:

```bash
go mod tidy
```

Do not run `go mod tidy` casually in large repos unless dependency cleanup is intended; it can create unrelated diffs.
