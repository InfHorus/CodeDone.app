# PHP / Laravel

Use this skill when the task touches PHP, Composer, Laravel, WordPress-aware PHP, public_html/cPanel-style apps, Apache/PHP hosting, sessions, PDO, Blade, migrations, queues, or PHP runtime errors.

PHP projects often mix legacy and modern code. Preserve existing structure and be careful with security-sensitive boundaries.

## Activation

Use for:

- `.php`, `composer.json`, `composer.lock`, `artisan`, Laravel apps, WordPress files, Blade templates, PHP sessions, PDO/MySQL, Apache `.htaccess`, public_html deployments, PHP-FPM/runtime issues.

Do not use for:

- JavaScript inside a PHP app unless PHP/backend behavior is part of the task.

## Core Rule

Respect the project’s framework level. Do not rewrite legacy PHP into Laravel, do not bypass Laravel conventions in Laravel apps, and do not modify WordPress core files.

## 80/20 PHP Workflow

1. Identify raw PHP vs Laravel vs WordPress/plugin/theme.
2. Check PHP version and Composer dependencies.
3. Preserve routing, autoloading, and hosting assumptions.
4. Validate input, escape output, parameterize SQL.
5. Keep secrets in `.env`/server config, not code.
6. Avoid changing `composer.lock` unless dependencies changed intentionally.
7. Run syntax/tests/framework commands when available.

## Composer

Use Composer for modern PHP dependencies.

Commands:

```bash
composer install
composer update vendor/package
composer dump-autoload
composer test
```

Avoid running broad `composer update` unless requested; it can upgrade many packages and create risky diffs.

## Raw PHP Safety

Use strict types in new isolated PHP files when compatible:

```php
<?php
declare(strict_types=1);
```

Validate request input:

```php
$email = filter_input(INPUT_POST, 'email', FILTER_VALIDATE_EMAIL);
if ($email === false || $email === null) {
    http_response_code(400);
    exit('Invalid email');
}
```

Escape output:

```php
<?= htmlspecialchars($name, ENT_QUOTES, 'UTF-8') ?>
```

Never echo raw user input into HTML.

## SQL / PDO

Use prepared statements:

```php
$stmt = $pdo->prepare('SELECT * FROM users WHERE email = :email');
$stmt->execute(['email' => $email]);
$user = $stmt->fetch(PDO::FETCH_ASSOC);
```

Avoid:

```php
$pdo->query("SELECT * FROM users WHERE email = '$email'");
```

For dynamic columns/sorts, whitelist identifiers. Values can be parameters; identifiers usually cannot.

## Sessions and Auth

For session login flows:

```php
session_regenerate_id(true);
$_SESSION['user_id'] = $user['id'];
```

Rules:

- Regenerate session ID after login.
- Do not store passwords or sensitive tokens in session unnecessarily.
- Use password hashing APIs:

```php
$hash = password_hash($password, PASSWORD_DEFAULT);
$isValid = password_verify($password, $hash);
```

Do not invent custom password hashing.

## Laravel Structure

Follow Laravel conventions:

```txt
app/Http/Controllers
app/Models
app/Services
app/Jobs
app/Policies
routes/web.php
routes/api.php
database/migrations
resources/views
```

Keep controllers thin:

```php
public function store(StoreUserRequest $request): JsonResponse
{
    $user = $this->users->create($request->validated());
    return response()->json(UserResource::make($user), 201);
}
```

Use Form Requests for non-trivial validation when the project already uses them.

## Laravel Eloquent

Avoid mass assignment vulnerabilities. Define `$fillable` or guarded behavior deliberately:

```php
protected $fillable = ['name', 'email'];
```

Use eager loading to prevent N+1:

```php
$posts = Post::query()->with('author')->latest()->paginate(20);
```

Do not fix every relation by making it globally eager. Load what the endpoint needs.

Use pagination for list endpoints, not unbounded `all()`:

```php
User::query()->where('team_id', $teamId)->paginate(50);
```

## Laravel Migrations

Migration example:

```php
Schema::table('users', function (Blueprint $table) {
    $table->string('timezone')->nullable()->after('email');
});
```

Consider existing rows, defaults, indexes, and rollback:

```php
public function down(): void
{
    Schema::table('users', function (Blueprint $table) {
        $table->dropColumn('timezone');
    });
}
```

Do not edit old migrations in an already-shipped app unless the project explicitly treats migrations as disposable.

## Laravel Config and Env

Use config wrappers, not raw `env()` throughout app code:

```php
// config/services.php
'stripe' => [
    'secret' => env('STRIPE_SECRET'),
]
```

```php
config('services.stripe.secret')
```

This matters because Laravel config caching can make scattered `env()` calls unreliable.

## Queues and Jobs

Use jobs for slow external calls, emails, media processing, and retryable work.

```php
SendInvoiceEmail::dispatch($invoice);
```

Make jobs idempotent when retries are possible. Do not assume a queued job runs exactly once.

## Blade

Blade escapes by default:

```blade
{{ $name }}
```

Raw output is dangerous:

```blade
{!! $html !!}
```

Only use raw output for trusted/sanitized HTML.

## WordPress Awareness

If working in WordPress:

- Do not edit WordPress core files.
- Prefer child themes for theme overrides.
- Use hooks/actions/filters.
- Sanitize input and escape output with WP helpers.
- Use nonces for forms/actions.
- Use `$wpdb->prepare()` for SQL.

Example:

```php
$rows = $wpdb->get_results(
    $wpdb->prepare('SELECT * FROM custom_table WHERE user_id = %d', $userId)
);
```

## File Uploads

Validate upload size, MIME/content, extension, storage path, and authorization.

Avoid trusting original filenames. Generate safe names:

```php
$path = $request->file('avatar')->store('avatars', 'public');
```

Do not place executable user uploads in public PHP-executable directories.

## Apache / public_html Notes

Be careful with `.htaccess` and document roots.

Laravel should point the web root to `public/`, not the project root. If forced into shared hosting, protect `.env`, `vendor/`, and app files from direct web access.

## Common AI Mistakes

Avoid these:

- Echoing user input without `htmlspecialchars` or Blade escaping.
- Building SQL with string concatenation.
- Running broad `composer update` for a small fix.
- Editing `vendor/` or WordPress core.
- Putting secrets in committed PHP files.
- Using `env()` directly all over Laravel app code.
- Returning Eloquent models with sensitive fields by accident.
- Using `Model::all()` for large lists.
- Ignoring CSRF/nonces on state-changing web forms.
- Adding a new route but forgetting middleware/auth.
- Breaking public_html hosting assumptions with framework-style paths.
- Uploading files to executable public directories.

## Verification

Common commands:

```bash
php -l path/to/file.php
composer test
vendor/bin/phpunit
vendor/bin/pest

# Laravel
php artisan test
php artisan migrate
php artisan route:list
php artisan config:clear
php artisan config:cache

# Static analysis if present
vendor/bin/phpstan analyse
vendor/bin/psalm
```

Use the project’s actual tooling. If no tests exist, at least run PHP syntax checks on touched files when possible.
