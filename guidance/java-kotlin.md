# Java / Kotlin

Use this skill when the task touches Java, Kotlin, JVM backends, Spring Boot, Android Kotlin/Java, Maven, Gradle, JPA/Hibernate, JVM tests, or JVM build/runtime errors.

Java and Kotlin share the JVM ecosystem, but keep language-specific idioms intact.

## Activation

Use for:

- `.java`, `.kt`, `.kts`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `settings.gradle`, `gradlew`, Spring Boot, Android projects, Maven/Gradle, JPA/Hibernate, JVM stack traces.
- REST APIs, services, repositories, entities, DTOs, dependency injection, coroutines, tests, build/dependency errors.

Do not use for:

- JavaScript/TypeScript despite Gradle/Vite tooling existing nearby.
- C#/.NET even if syntax looks similar.

## Core Rule

Respect the existing architecture: controller/service/repository boundaries, DTO/entity separation, dependency injection style, build tool, package layout, and test framework. Do not “simplify” by bypassing established layers.

## 80/20 JVM Workflow

1. Identify Maven vs Gradle and Java/Kotlin versions.
2. Read package structure and existing patterns before adding classes.
3. Keep public API models, persistence entities, and internal domain objects separate when the project already does.
4. Prefer constructor injection.
5. Preserve nullability contracts.
6. Avoid blocking calls in coroutine/reactive code.
7. Update tests and build files only when necessary.
8. Run targeted tests/build if possible.

## Build Tools

Do not mix Maven and Gradle.

Maven commands:

```bash
mvn test
mvn package
mvn spring-boot:run
```

Gradle commands:

```bash
./gradlew test
./gradlew build
./gradlew bootRun
```

Use the wrapper (`./gradlew`) when present. Do not assume globally installed Gradle.

## Project Structure

Common Spring layout:

```txt
src/main/java/com/example/app/
  controller/
  service/
  repository/
  entity/
  dto/
src/test/java/com/example/app/
```

Common Android layout:

```txt
app/src/main/java/...
app/src/main/res/...
build.gradle.kts
AndroidManifest.xml
```

Follow the existing package conventions exactly.

## Spring Boot

Keep controllers thin and move business logic to services.

Controller pattern:

```java
@RestController
@RequestMapping("/api/users")
class UserController {
  private final UserService users;

  UserController(UserService users) {
    this.users = users;
  }

  @GetMapping("/{id}")
  UserDto getUser(@PathVariable Long id) {
    return users.getUser(id);
  }
}
```

Avoid putting database queries, transaction-heavy logic, or complex validation directly in controllers.

## Dependency Injection

Prefer constructor injection:

```java
@Service
class BillingService {
  private final InvoiceRepository invoices;

  BillingService(InvoiceRepository invoices) {
    this.invoices = invoices;
  }
}
```

Avoid field injection:

```java
@Autowired
private InvoiceRepository invoices; // avoid unless project already uses it everywhere
```

## DTOs vs Entities

Do not expose JPA entities directly from controllers unless the project already intentionally does that.

Reasons:

- Lazy-loading surprises.
- Infinite JSON recursion.
- Sensitive/internal fields leaked.
- API shape coupled to database schema.

Use DTOs or response records:

```java
public record UserDto(Long id, String email, String name) {}
```

## JPA / Hibernate

Be careful with lazy loading, transactions, and N+1 queries.

Common rules:

- Use repositories for persistence access.
- Put write operations in `@Transactional` service methods.
- Avoid accessing lazy relations after the transaction closes.
- Use fetch joins/entity graphs when needed.
- Do not solve N+1 by switching everything to eager loading.

Example:

```java
@Transactional
public OrderDto createOrder(CreateOrderRequest request) {
  var order = new Order(...);
  repository.save(order);
  return mapper.toDto(order);
}
```

## Kotlin Idioms

Use Kotlin idioms in Kotlin files:

```kotlin
data class UserDto(
    val id: Long,
    val email: String,
    val name: String,
)
```

Prefer immutable `val` where possible. Respect nullable types:

```kotlin
val name: String? = user.name
```

Do not silence nullability by spraying `!!`. Handle null intentionally:

```kotlin
val user = repository.findById(id).orElseThrow { NotFoundException("User not found") }
```

## Coroutines / Reactive Code

Do not block event loops or coroutine dispatchers with blocking I/O.

Avoid:

```kotlin
runBlocking { service.call() } // wrong in request-handling paths
```

In coroutine code, use `suspend` all the way where the framework supports it. In Reactor/WebFlux, do not call `.block()` in handlers.

## Error Handling

Do not swallow exceptions.

Prefer domain-specific exceptions mapped centrally:

```java
@ResponseStatus(HttpStatus.NOT_FOUND)
class NotFoundException extends RuntimeException {
  NotFoundException(String message) { super(message); }
}
```

For APIs, use a consistent error response strategy. Do not leak stack traces to clients.

## Validation

Use Bean Validation where the project uses Spring/Java validation:

```java
public record CreateUserRequest(
  @Email @NotBlank String email,
  @NotBlank String name
) {}
```

And enable validation in controllers:

```java
@PostMapping
UserDto create(@Valid @RequestBody CreateUserRequest request) { ... }
```

Validation in the controller does not replace database constraints.

## Android Notes

For Android Kotlin/Java:

- Respect lifecycle; avoid leaking Activity/Context.
- Do not do network or disk I/O on the main thread.
- Use ViewModel/state patterns if already present.
- Keep permissions explicit and handle denial paths.
- Prefer resource strings over hardcoded UI text when the project is localized.

Kotlin coroutine example:

```kotlin
viewModelScope.launch {
    val result = repository.loadUser()
    _state.value = State.Ready(result)
}
```

## Testing

Common frameworks:

- JUnit 5
- Mockito / MockK
- Spring Boot Test
- Testcontainers
- AssertJ

Prefer focused tests for service logic and integration tests for persistence/controller behavior when the project already has that style.

Commands:

```bash
mvn test
./gradlew test
./gradlew connectedAndroidTest
```

## Common AI Mistakes

Avoid these:

- Adding Lombok to a project that does not use it.
- Switching Maven to Gradle or Gradle Groovy to Kotlin DSL without request.
- Returning JPA entities directly from new controllers.
- Creating circular dependencies between services.
- Using field injection in otherwise constructor-injected projects.
- Ignoring nullability in Kotlin or using `!!` to force success.
- Calling `.get()` on `Optional` without handling absence.
- Marking everything `@Transactional` without reason.
- Fixing N+1 by making every relation eager.
- Blocking in WebFlux/coroutines.
- Editing generated build output instead of source/build files.
- Adding dependencies for trivial code the JDK already covers.

## Verification

Use the project wrapper and targeted tests where possible:

```bash
# Maven
mvn test
mvn -q -DskipTests package

# Gradle
./gradlew test
./gradlew build

# Spring Boot
./gradlew bootRun
mvn spring-boot:run

# Android
./gradlew assembleDebug
./gradlew testDebugUnitTest
```

If build/test cannot run, inspect imports, package names, bean wiring, and build-file consistency carefully.
