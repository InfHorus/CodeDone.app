---
name: cybersecurity
description: Defensive cybersecurity guidance for secure code review, threat modeling, vulnerability discovery, CVE-aware dependency checks, hardening, and fix-oriented white-hat analysis. Use only when the task explicitly involves security, audit, auth, input validation, exposed services, dependencies, secrets, or abuse resistance.
license: Complete terms in LICENSE.txt
---

# Cybersecurity Skill

Use this skill when the user explicitly asks for security review, vulnerability assessment, hardening, threat modeling, CVE/dependency review, abuse resistance, authentication/authorization checks, secrets handling, exposed service review, or secure implementation.

Also use it when the current code touches high-risk surfaces: auth, sessions, permissions, payments, file upload/download, deserialization, templates, SQL/ORM queries, command execution, SSRF-capable outbound requests, webhooks, cryptography, API keys, admin panels, multi-tenant data, network services, or sandboxing.

Do **not** use it for ordinary implementation just because every app has security concerns. Avoid context bloat.

## Defensive Boundary

Operate as a white-hat reviewer trying to find failure points so they can be fixed.

Allowed output style:

- Explain risk and exploitability at a high level.
- Show safe validation, hardening, and remediation code.
- Provide non-destructive local checks.
- Recommend dependency upgrades, config changes, tests, and logging.
- Produce prioritized findings with severity, evidence, impact, and fix.

Avoid:

- Weaponized exploit chains.
- Payload lists for live targets.
- Instructions for stealth, persistence, evasion, credential theft, or post-exploitation.
- Scanning third-party systems without authorization.

## Security Review 80/20

### 1. Map trust boundaries

Identify:

- User-controlled inputs: body, query, path params, headers, cookies, uploaded files, webhooks, OAuth callbacks, environment variables, queue messages.
- Privileged actions: admin routes, billing, account changes, data export, password reset, API key creation.
- Sensitive assets: secrets, tokens, PII, financial data, source code, logs, backups.
- External boundaries: databases, object storage, internal HTTP services, shell commands, third-party APIs.

Then ask: “What can an untrusted user influence, and what does the system do with it?”

### 2. Authorization before validation polish

Broken access control is often more damaging than malformed input. Check:

- Object-level authorization: can user A access user B's object by changing an ID?
- Function-level authorization: can normal users hit admin/moderator endpoints?
- Tenant scoping: does every query constrain by tenant/account/org?
- Server-side enforcement: is permission checked on the server, not only UI?
- Default deny: new routes/actions require explicit access control.

Good pattern:

```ts
const invoice = await db.invoice.findFirst({
  where: { id: invoiceId, accountId: session.accountId }
});
if (!invoice) throw new NotFoundError();
```

Bad pattern:

```ts
const invoice = await db.invoice.findUnique({ where: { id: invoiceId } });
```

### 3. Input validation and injection

Check all places where input enters interpreters:

- SQL/ORM raw queries.
- Shell commands.
- HTML/templates/Markdown rendering.
- File paths.
- LDAP/XML/YAML/deserialization.
- Regexes with untrusted patterns.
- Dynamic imports/eval/plugins.

Prefer allowlists and parameterization.

```python
# Good: no shell, explicit args.
subprocess.run(["ffmpeg", "-i", input_path, output_path], check=True)
```

```sql
-- Good: parameterized through the driver/ORM, not string concatenation.
SELECT * FROM users WHERE email = ?
```

### 4. File upload/download safety

Check:

- File type validation by content, not only extension.
- Size limits before buffering fully in memory.
- Randomized storage names.
- Path traversal prevention.
- Private bucket/default deny unless public is intentional.
- Malware scanning for risky business contexts.
- Image/document processing isolated from privileged systems.

```python
target = (UPLOAD_ROOT / safe_generated_name).resolve()
if not str(target).startswith(str(UPLOAD_ROOT.resolve())):
    raise ValueError("invalid path")
```

### 5. SSRF and outbound requests

Any feature that fetches URLs can hit internal infrastructure if not constrained.

Check:

- URL previewers, webhooks, importers, PDF/image fetchers, AI browsing/RAG ingestion.
- Private IP ranges, link-local metadata services, localhost, DNS rebinding.
- Redirect handling.
- Protocol allowlist: usually only `https`.
- Egress network policy when possible.

Good pattern:

```txt
Allowlist destination hosts when business logic permits it.
Resolve and block private/link-local/loopback IPs after redirects.
Set short timeouts and response size limits.
```

### 6. Authentication and session safety

Check:

- Password reset token entropy, expiry, one-time use.
- Session cookie flags: HttpOnly, Secure, SameSite.
- MFA bypasses and recovery flows.
- OAuth redirect URI validation.
- Login rate limits and credential stuffing controls.
- API key hashing and scoped permissions.

```ts
cookie.serialize("session", token, {
  httpOnly: true,
  secure: true,
  sameSite: "lax",
  path: "/",
  maxAge: 60 * 60 * 24 * 7
});
```

### 7. Secrets and configuration

Check:

- Secrets committed to repo/history.
- Secrets printed in logs/errors.
- `.env` handling in Docker/CI.
- Overly broad cloud credentials.
- Public S3/object storage buckets.
- Debug mode enabled in production.
- CORS too permissive with credentials.

Never add real secrets to examples. Use placeholders.

### 8. Dependency and CVE review

Check:

- Lockfiles and dependency age.
- Direct and transitive vulnerable packages.
- Abandoned packages.
- Runtime images/base OS CVEs.
- Whether a patched version introduces breaking changes.
- Whether mitigations are temporary or complete.

Commands depend on ecosystem:

```bash
npm audit
pnpm audit
pip-audit
safety check
composer audit
dotnet list package --vulnerable
go list -m -u all
govulncheck ./...
cargo audit
trivy fs .
```

### 9. Logging and monitoring

Check:

- Security events logged: login failures, token creation, permission denials, admin actions, payment/webhook failures.
- Logs do not include secrets, tokens, passwords, raw auth headers, or excessive PII.
- Alerts exist for unusual volume, repeated failures, and high-risk actions.
- Error responses do not leak stack traces in production.

### 10. Abuse resistance

Check:

- Rate limits on auth, expensive APIs, upload endpoints, AI endpoints, search, password reset, and webhooks.
- Quotas per user/org/API key/IP where appropriate.
- Idempotency keys for payments and critical retries.
- Replay protection for signed webhooks.
- Backpressure for queues/background jobs.

## CVE Lessons Without Exploit Playbooks

Use CVEs as pattern examples, not as payload instructions.

### CVE-2021-44228 — Log4Shell / Log4j RCE

Lesson: untrusted data reaching unexpected interpreters can become catastrophic, especially through transitive dependencies. Defensive checks:

- Inventory direct and transitive dependencies.
- Keep lockfiles and SBOM-like records.
- Upgrade vulnerable components, not just sanitize input.
- Treat logging, templating, serialization, and plugin systems as security-sensitive interpreters.
- Add dependency scanning to CI.

### CVE-2023-34362 — MOVEit Transfer SQL injection

Lesson: exposed internet-facing enterprise software with injection flaws can become mass-compromise infrastructure. Defensive checks:

- Parameterize every database path, including admin/import/search/report endpoints.
- Review custom query builders and raw SQL.
- Add WAF/log detection as defense-in-depth, not a replacement for patching.
- Patch externally exposed products quickly and verify compromise indicators.

### CVE-2022-22965 — Spring4Shell

Lesson: framework binding and object-mapping features can expose dangerous internals. Defensive checks:

- Avoid broad automatic binding into sensitive objects.
- Use narrow DTOs/request models.
- Keep framework versions current.
- Add tests for over-posting/mass-assignment.

### CVE-2019-19781 — Citrix ADC/Gateway path traversal/RCE class

Lesson: edge devices and gateways are high-value targets. Defensive checks:

- Patch perimeter appliances quickly.
- Minimize exposed management surfaces.
- Review path normalization and access control around static/template resources.
- Monitor edge logs for suspicious requests and unexpected file writes.

## Finding Failure Points

Use this mental checklist:

```txt
Can I change an ID and see someone else's data?
Can I call an admin action as a non-admin?
Can I make the server fetch/connect to an internal address?
Can I upload a file that executes, overwrites, or escapes its directory?
Can I inject into SQL, shell, template, regex, markdown, XML, YAML, or deserialization?
Can I replay a webhook/payment/action?
Can I bypass validation by using another route, API version, or background job?
Can I cause high CPU/memory/storage cost cheaply?
Can I leak secrets through logs, errors, source maps, backups, or public buckets?
Can a stale dependency/base image carry a known CVE?
```

## Finding Format

Use concise, actionable findings:

```md
### [High] Missing tenant scope on invoice lookup

**Evidence:** `getInvoice(id)` queries by `id` only.
**Impact:** A user who knows or guesses another invoice ID may read cross-account billing data.
**Fix:** Add `accountId/orgId` scope to the query and return 404 for missing/unauthorized objects.
**Regression test:** User A cannot fetch User B's invoice by ID.
```

## Fix-Oriented Snippets

### Tenant-scoped query

```ts
await db.project.findFirst({
  where: { id: projectId, organizationId: session.organizationId }
});
```

### Webhook signature before parsing trust

```ts
const valid = verifySignature(rawBody, signature, webhookSecret);
if (!valid) return new Response("invalid signature", { status: 401 });
```

### Safe redirect

```ts
const allowed = new Set(["/dashboard", "/settings"]);
const next = allowed.has(inputNext) ? inputNext : "/dashboard";
```

### Rate-limit high-cost endpoints

```txt
Key by user/account/API key first, IP second.
Use stricter limits for auth, password reset, uploads, AI inference, exports, and admin actions.
```

## Common AI Mistakes

- Treating authentication as authorization.
- Checking permissions in UI only.
- Forgetting tenant/org scoping in database queries.
- Concatenating shell/SQL/template strings with user input.
- Allowing arbitrary outbound URLs without SSRF controls.
- Trusting uploaded file names, MIME types, or extensions.
- Logging tokens/secrets in debug output.
- Adding permissive CORS with credentials.
- Storing API keys in plaintext instead of hashing when they must be verified later.
- Ignoring lockfiles and transitive dependencies.
- Giving generic “sanitize input” advice instead of concrete allowlists and parameterization.
- Reporting scary findings without evidence, impact, and a fix.

## Verification

Prefer safe, local, authorized checks:

```bash
# Dependency / CVE checks
npm audit
pip-audit
composer audit
dotnet list package --vulnerable
govulncheck ./...
trivy fs .

# Secret scanning
gitleaks detect
trufflehog filesystem .

# Static analysis where configured
semgrep scan
bandit -r .
```

If tools are unavailable, perform manual review and state what was not verified.
