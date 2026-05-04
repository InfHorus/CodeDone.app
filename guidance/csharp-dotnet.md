# C# / .NET

Use this skill for C#, .NET, ASP.NET Core, Entity Framework, NuGet, MSBuild, MAUI/WPF/WinForms, Unity C# scripts, background services, CLI tools, and managed application code. Keep changes idiomatic to the existing project and avoid importing web/backend patterns into Unity or desktop apps unless the repo already uses them.

---

## Routing Rules

Use when the repo or task includes any of:

- `.cs`, `.csproj`, `.sln`, `.props`, `.targets`, `global.json`, `Directory.Build.props`, `Directory.Build.targets`
- `Program.cs`, `Startup.cs`, `appsettings.json`, `launchSettings.json`
- ASP.NET Core, Minimal APIs, MVC, Razor, Blazor, SignalR, gRPC
- Entity Framework Core, Dapper, LINQ-heavy data access
- NuGet, MSBuild, dotnet CLI, xUnit, NUnit, MSTest
- Unity C# scripts, Godot C# scripts, MAUI, WPF, WinForms, Avalonia, background workers, Windows services

Do not use for native C/C++ work. Use `cpp-native` for `.c/.cpp/.h/.hpp`, CMake, compiler/linker errors, native libraries, or ABI issues.

---

## Operating Standard

Before editing, infer the .NET project shape from files, not assumptions:

- Target framework: `net8.0`, `net9.0`, `netstandard2.0`, Unity profile, MAUI targets, etc.
- App type: ASP.NET Core API, MVC/Razor, Blazor, console/CLI, worker service, desktop app, Unity game, library.
- Dependency style: built-in DI, options pattern, EF Core, MediatR, Dapper, repository pattern, minimal APIs, controllers.
- Nullable mode and analyzers: respect `<Nullable>enable</Nullable>`, warnings-as-errors, style rules.
- Existing architecture: do not introduce a new layering pattern for a small change.

Prefer small, idiomatic patches. Do not upgrade target frameworks, rewrite project files, replace dependency injection architecture, or change serialization/database behavior unless explicitly required.

---

## 80/20 Rules

### 1. Respect nullable reference types

If nullable is enabled, make nullability meaningful. Do not silence with `!` unless the invariant is truly guaranteed.

```csharp
public sealed class UserService
{
    public UserDto? FindById(Guid id)
    {
        // null means not found
        return null;
    }
}
```

Prefer validation at boundaries:

```csharp
if (string.IsNullOrWhiteSpace(request.Email))
    return Results.BadRequest("Email is required.");
```

Avoid:

```csharp
var user = await db.Users.FindAsync(id);
return user!.Email; // hides a possible not-found bug
```

### 2. Use async correctly

Use `async` all the way for I/O. Do not block async work with `.Result`, `.Wait()`, or `GetAwaiter().GetResult()` in app code.

```csharp
public async Task<User?> GetUserAsync(Guid id, CancellationToken ct)
{
    return await db.Users.FirstOrDefaultAsync(x => x.Id == id, ct);
}
```

Pass `CancellationToken` through APIs, database calls, HTTP calls, and background services when available.

```csharp
app.MapGet("/users/{id:guid}", async (Guid id, AppDbContext db, CancellationToken ct) =>
{
    var user = await db.Users.FindAsync([id], ct);
    return user is null ? Results.NotFound() : Results.Ok(user);
});
```

### 3. Keep dependency injection idiomatic

Do not manually instantiate services that need configured dependencies. Use DI according to existing project style.

```csharp
builder.Services.AddScoped<UserService>();
builder.Services.AddHttpClient<PaymentClient>();
```

Constructor injection:

```csharp
public sealed class UserService(AppDbContext db, ILogger<UserService> logger)
{
    public async Task<User?> FindAsync(Guid id, CancellationToken ct)
    {
        logger.LogDebug("Loading user {UserId}", id);
        return await db.Users.FindAsync([id], ct);
    }
}
```

Do not inject `IServiceProvider` and service-locate unless the project already has a deliberate dynamic-resolution pattern.

### 4. Keep configuration out of code

Use `appsettings.json`, environment variables, user secrets, Key Vault, or the project’s existing config system.

```csharp
builder.Services.Configure<AuthOptions>(builder.Configuration.GetSection("Auth"));

public sealed class AuthOptions
{
    public string Issuer { get; init; } = "";
    public string Audience { get; init; } = "";
}
```

Never hardcode secrets, API keys, connection strings, signing keys, or production URLs into source.

### 5. Keep DTOs separate from database entities when crossing API boundaries

Do not return EF entities directly from public APIs unless the project intentionally does so.

```csharp
public sealed record UserDto(Guid Id, string Email, string DisplayName);

static UserDto ToDto(User user) => new(user.Id, user.Email, user.DisplayName);
```

This avoids leaking internal columns, navigation cycles, lazy loading surprises, and serialization instability.

### 6. Use LINQ carefully

LINQ against `IQueryable` becomes SQL for EF Core. Not every C# expression translates.

Good:

```csharp
var users = await db.Users
    .Where(u => u.IsActive)
    .OrderBy(u => u.Email)
    .Select(u => new UserDto(u.Id, u.Email, u.DisplayName))
    .ToListAsync(ct);
```

Avoid pulling the database into memory too early:

```csharp
// Bad for large tables
var users = db.Users.ToList().Where(u => ExpensiveLocalCheck(u));
```

Use `AsNoTracking()` for read-only EF queries when appropriate.

### 7. Handle errors at the right layer

Do not swallow exceptions silently. Log with context and return appropriate API/UI results.

```csharp
try
{
    await service.ProcessAsync(command, ct);
    return Results.NoContent();
}
catch (NotFoundException)
{
    return Results.NotFound();
}
catch (ValidationException ex)
{
    return Results.BadRequest(ex.Message);
}
```

Do not catch `Exception` just to return `false` unless the codebase uses that style and logs elsewhere.

---

## ASP.NET Core / APIs

High-value practices:

- Validate request models at the boundary.
- Use typed DTOs, not loose dictionaries, for normal APIs.
- Use route constraints for IDs where useful.
- Return consistent status codes.
- Keep business logic out of controllers/minimal route lambdas when it grows.
- Use auth/authorization policies rather than ad-hoc checks scattered everywhere.
- Avoid logging secrets, tokens, full request bodies, or sensitive PII.

Minimal API example:

```csharp
app.MapPost("/projects", async (
    CreateProjectRequest request,
    ProjectService projects,
    CancellationToken ct) =>
{
    if (string.IsNullOrWhiteSpace(request.Name))
        return Results.BadRequest("Name is required.");

    var project = await projects.CreateAsync(request, ct);
    return Results.Created($"/projects/{project.Id}", project);
})
.RequireAuthorization();

public sealed record CreateProjectRequest(string Name);
```

Controller style: follow existing controller/service patterns instead of converting to minimal APIs mid-task.

---

## Entity Framework Core / Data Access

Do not change migrations casually. Database changes must be explicit.

Common commands:

```bash
dotnet ef migrations add <Name>
dotnet ef database update
dotnet test
```

Practical rules:

- Use async EF calls in request paths.
- Use `AsNoTracking()` for read-only queries.
- Project to DTOs instead of returning entities.
- Avoid N+1 queries; use projection or `Include` deliberately.
- Do not call `SaveChangesAsync` inside tight loops.
- Respect existing transaction boundaries.

```csharp
var order = await db.Orders
    .AsNoTracking()
    .Where(o => o.Id == id)
    .Select(o => new OrderDto(
        o.Id,
        o.CustomerEmail,
        o.Items.Select(i => new OrderItemDto(i.Sku, i.Quantity)).ToList()))
    .FirstOrDefaultAsync(ct);
```

---

## Unity / Game C#

Unity C# is not normal ASP.NET C#.

High-value practices:

- Do not use heavy allocations in `Update`, `FixedUpdate`, or hot gameplay loops.
- Cache component references when used often.
- Use `SerializeField` for inspector wiring without making fields public.
- Use coroutines or async carefully; Unity object lifetime can invalidate continuations.
- Touch Unity APIs from the main thread unless using Unity-approved job/thread patterns.
- Prefer `Time.deltaTime` for frame-rate-independent movement and `FixedUpdate` for physics.

```csharp
public sealed class Mover : MonoBehaviour
{
    [SerializeField] private float speed = 5f;
    private Rigidbody rb = null!;

    private void Awake()
    {
        rb = GetComponent<Rigidbody>();
    }

    private void FixedUpdate()
    {
        var input = new Vector3(Input.GetAxisRaw("Horizontal"), 0f, Input.GetAxisRaw("Vertical"));
        rb.MovePosition(rb.position + input.normalized * speed * Time.fixedDeltaTime);
    }
}
```

Avoid repeated `GetComponent`, LINQ, string concatenation, and new allocations in hot paths unless negligible for the project.

---

## Desktop / MAUI / WPF / WinForms

High-value practices:

- Keep UI thread responsive; move I/O and long work off the UI thread.
- Marshal updates back to the UI thread using the framework’s dispatcher/invocation model.
- Follow existing MVVM or code-behind style; do not introduce MVVM in one random file unless requested.
- Validate user input before file/network/database operations.
- Dispose streams, timers, subscriptions, and unmanaged handles.

WPF-style command work should avoid blocking:

```csharp
private async Task LoadAsync()
{
    IsLoading = true;
    try
    {
        Items = await service.LoadItemsAsync(CancellationToken.None);
    }
    finally
    {
        IsLoading = false;
    }
}
```

---

## Libraries and Public APIs

For reusable libraries:

- Treat public types and method signatures as contracts.
- Avoid breaking binary/source compatibility unless requested.
- Keep exceptions documented or obvious.
- Avoid adding framework-specific dependencies to general-purpose libraries.
- Use `internal` for implementation details.

```csharp
public interface IClock
{
    DateTimeOffset UtcNow { get; }
}

internal sealed class SystemClock : IClock
{
    public DateTimeOffset UtcNow => DateTimeOffset.UtcNow;
}
```

---

## NuGet / Project Files

Respect the existing dependency and project structure.

Useful commands:

```bash
dotnet restore
dotnet build
dotnet test
dotnet format

dotnet add package <PackageName>
dotnet list package --outdated
```

Do not upgrade packages or target frameworks opportunistically. Do not edit generated files. Be careful with central package management:

```txt
Directory.Packages.props
Directory.Build.props
Directory.Build.targets
global.json
```

If these exist, update the central file rather than adding random versions in `.csproj`.

---

## Security-Sensitive .NET Code

Use extra caution when touching:

- authentication, authorization, JWT/cookies, sessions
- password storage/reset flows
- file uploads/downloads
- payment webhooks
- SQL/raw queries
- SSRF-prone outbound HTTP
- deserialization, reflection, plugins
- CORS, CSRF, rate limits, admin endpoints

Practical rules:

- Use parameterized queries; never concatenate SQL with user input.
- Validate file names, extensions, content length, and storage paths.
- Verify webhook signatures using raw body when required by provider.
- Set secure cookie/auth defaults according to the app’s deployment.
- Avoid permissive CORS unless explicitly needed.
- Do not log tokens, passwords, API keys, or full auth headers.

Dapper parameter example:

```csharp
var user = await connection.QuerySingleOrDefaultAsync<UserDto>(
    "select id, email, display_name from users where id = @Id",
    new { Id = id });
```

---

## Testing and Verification

Choose commands from the repo:

```bash
dotnet restore
dotnet build
dotnet test
```

Target specific projects when the solution is large:

```bash
dotnet test tests/MyApp.Tests/MyApp.Tests.csproj
```

Common test frameworks:

```txt
xUnit: [Fact], [Theory]
NUnit: [Test]
MSTest: [TestMethod]
ASP.NET: WebApplicationFactory<TEntryPoint>
```

Do not claim tests passed unless run. If unable to run, state the exact reason.

---

## Common AI Mistakes to Avoid

- Mixing C#/.NET with C/C++ advice.
- Blocking async code using `.Result` or `.Wait()`.
- Ignoring nullable reference warnings or hiding them with `!`.
- Returning EF entities directly from APIs when DTOs are expected.
- Adding new architecture layers for a tiny feature.
- Adding package versions in the wrong place when central package management is used.
- Upgrading target frameworks or NuGet packages without being asked.
- Forgetting to register new services in DI.
- Creating a new `HttpClient` manually per request instead of using `IHttpClientFactory` where configured.
- Using LINQ that EF cannot translate, or calling `ToList()` too early.
- Doing heavy work or allocations in Unity `Update`/`FixedUpdate` loops.
- Touching UI state from a background thread in desktop apps.
- Swallowing exceptions without logging or user-visible failure.
- Hardcoding secrets, connection strings, or environment-specific URLs.

---

## Output Standard

When finishing C#/.NET work, report:

- What changed and why.
- Whether public APIs, database schema/migrations, project files, or dependencies changed.
- Build/test commands run, or exact reason they were not run.
- Any nullable, async, DI, EF, Unity, or deployment assumptions.
