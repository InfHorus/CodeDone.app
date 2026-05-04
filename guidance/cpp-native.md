# C / C++ Native

Use this skill for C and C++ projects: native libraries, CLI tools, engines, game code, performance-sensitive code, embedded-ish code, compiler/linker issues, CMake/Make/MSVC/Clang/GCC work, and cross-platform native integration. Keep changes conservative: native code is easy to break through ABI, lifetime, build, or platform assumptions.

---

## Routing Rules

Use when the repo or task includes any of:

- `.c`, `.h`, `.cpp`, `.cc`, `.cxx`, `.hpp`, `.hh`, `.hxx`, `.ixx`
- `CMakeLists.txt`, `Makefile`, `meson.build`, `BUILD`, `WORKSPACE`, `premake*.lua`, `.vcxproj`, `.sln` used for native code
- GCC, Clang, MSVC, MinGW, Ninja, Make, CMake, Meson, Bazel, vcpkg, Conan
- native addons, FFI/bindings, shared libraries, static libraries, game engine modules, engine plugins, low-level performance code
- compiler errors, linker errors, undefined symbols, include-path issues, ABI/platform issues

Do not use for C#/.NET just because the filename contains “C”. Use `csharp-dotnet` for `.cs`, `.csproj`, `.sln` managed projects, ASP.NET, Unity C#, NuGet, or .NET runtime behavior.

---

## Operating Standard

Before editing, infer the native project shape from files, not assumptions:

- Language mode: C vs C++, and actual standard: C99/C11/C17/C23, C++11/14/17/20/23.
- Compiler/toolchain: GCC, Clang, MSVC, MinGW, Emscripten, Android NDK, platform SDK.
- Build system: CMake, Make, Meson, Bazel, Visual Studio, premake, custom scripts.
- Binary type: executable, static lib, shared lib, plugin/module, native extension, embedded firmware.
- Platform constraints: Windows/Linux/macOS, x86/x64/ARM, DLL export rules, POSIX vs Win32 APIs.

Prefer minimal, local patches. Do not modernize an entire codebase while fixing a bug. Do not change public headers, ABI, exported symbols, build flags, or memory ownership contracts unless required.

---

## 80/20 Rules

### 1. Preserve ownership and lifetime contracts

Most native bugs are lifetime bugs. Before changing pointer/reference code, identify who owns the memory and who merely borrows it.

C++ preferred pattern:

```cpp
// Owns the object
std::unique_ptr<Session> session = std::make_unique<Session>(config);

// Borrows; never store beyond owner's lifetime unless documented
Session* current = session.get();
```

Avoid returning references/pointers to local objects:

```cpp
// Bad: dangling reference
const std::string& name() {
    std::string value = "player";
    return value;
}
```

In C, make allocation/free ownership explicit in names or comments:

```c
// Caller owns returned buffer and must free() it.
char *read_file_alloc(const char *path);

// Borrowed pointer; do not free.
const char *config_get_name(const struct config *cfg);
```

### 2. Prefer RAII in C++; be explicit in C

In C++, avoid manual `new`/`delete` unless integrating with an API that requires it.

```cpp
std::vector<std::uint8_t> bytes(size);
std::ifstream file(path, std::ios::binary);
```

In C, use single-exit cleanup for multi-step resource acquisition:

```c
int load_resource(const char *path) {
    FILE *f = fopen(path, "rb");
    if (!f) return -1;

    void *buf = malloc(4096);
    if (!buf) {
        fclose(f);
        return -1;
    }

    /* work */

    free(buf);
    fclose(f);
    return 0;
}
```

For more complex C cleanup, `goto cleanup` is acceptable when it prevents leaks and duplicated teardown.

### 3. Treat headers as contracts

Headers are dependency multipliers. Keep them stable and minimal.

- Do not put heavy includes in headers when a forward declaration works.
- Do not change public struct layout casually; it can break ABI.
- Keep implementation details in `.c/.cpp` files.
- Avoid `using namespace` in headers.
- Include what you use, but avoid include explosions.

C++ forward declaration example:

```cpp
// player.hpp
#pragma once
#include <memory>

class Inventory;

class Player {
public:
    explicit Player(std::unique_ptr<Inventory> inventory);
private:
    std::unique_ptr<Inventory> inventory_;
};
```

### 4. Do not “fix” linker errors by random build changes

Linker errors usually mean one of:

- A source file is not part of the target.
- A library is not linked to the target.
- Declaration and definition signatures differ.
- C vs C++ name mangling mismatch.
- Symbol visibility/export issue on shared libraries.
- Wrong architecture/configuration/debug-release mix.

CMake target-first pattern:

```cmake
add_library(core STATIC
    src/core.cpp
    src/session.cpp
)

target_include_directories(core PUBLIC include)
target_compile_features(core PUBLIC cxx_std_20)

target_link_libraries(app PRIVATE core)
```

Prefer target-scoped CMake over global flags:

```cmake
# Good
target_compile_options(core PRIVATE -Wall -Wextra)

# Avoid unless the project already uses it globally
add_compile_options(-Wall -Wextra)
```

### 5. Match the project’s standard and style

Do not introduce C++20 ranges into a C++14 codebase. Do not introduce exceptions into code compiled with exceptions disabled. Do not add RTTI-heavy patterns to no-RTTI engine code.

Check for:

```txt
- CMAKE_CXX_STANDARD / CMAKE_C_STANDARD
- /std:c++17, -std=c++17, -fno-exceptions, -fno-rtti
- warning level and warnings-as-errors
- existing formatting and naming conventions
```

### 6. Validate input and bounds manually

Native code should assume external input can be malformed.

C-style bounds check:

```c
int parse_packet(const uint8_t *data, size_t len) {
    if (!data || len < 4) return -1;

    uint16_t type = (uint16_t)data[0] | ((uint16_t)data[1] << 8);
    uint16_t size = (uint16_t)data[2] | ((uint16_t)data[3] << 8);

    if ((size_t)size > len - 4) return -1;
    return handle_payload(type, data + 4, size);
}
```

Avoid unsafe functions unless the project already wraps them carefully:

```txt
gets, strcpy, strcat, sprintf, scanf("%s"), unchecked memcpy/memmove, raw pointer arithmetic without length checks
```

### 7. Be careful with concurrency

Thread bugs are usually from shared mutable state, lifetime, or lock ordering.

C++ basics:

```cpp
std::mutex mutex_;
std::vector<Job> jobs_;

void push(Job job) {
    std::lock_guard<std::mutex> lock(mutex_);
    jobs_.push_back(std::move(job));
}
```

Do not hold locks while calling arbitrary callbacks, blocking I/O, or code that may re-enter the same system unless intentional.

Use atomics only for simple state. Do not replace a mutex with `std::atomic` unless the memory model is clearly correct.

### 8. Keep error handling consistent

C projects often use return codes. C++ projects may use exceptions, `std::optional`, `std::expected` if available, status objects, or engine-specific result types.

Do not mix styles randomly.

```cpp
std::optional<Config> load_config(const std::filesystem::path& path) {
    std::ifstream file(path);
    if (!file) return std::nullopt;
    // parse...
    return Config{};
}
```

For C APIs, return clear status and use out-params when the codebase does that:

```c
bool config_load(const char *path, struct config *out_config);
```

---

## C-Specific Guidance

Use C when the codebase is C, embedded-like, ABI-facing, or library-oriented. Keep it direct and predictable.

High-value practices:

- Initialize structs deliberately.
- Check all allocation and I/O results.
- Keep buffer length next to pointer.
- Avoid hidden ownership transfer.
- Use `const` for borrowed read-only data.
- Prefer fixed-width integers for binary formats: `uint32_t`, `int64_t`.
- Avoid macros where `static inline` works.

```c
struct buffer {
    uint8_t *data;
    size_t len;
};

void buffer_free(struct buffer *buf) {
    if (!buf) return;
    free(buf->data);
    buf->data = NULL;
    buf->len = 0;
}
```

Macro caution:

```c
// Prefer this when type-specific is fine
static inline int clamp_int(int v, int lo, int hi) {
    return v < lo ? lo : (v > hi ? hi : v);
}
```

---

## C++-Specific Guidance

Use modern C++ to reduce lifetime risk, not to show off.

High-value practices:

- Use RAII for resources.
- Prefer `std::vector`, `std::string`, `std::array`, `std::span` when available.
- Use `std::unique_ptr` for ownership, raw pointers/references for non-owning access.
- Use `std::move` only when ownership transfer is intended.
- Prefer `enum class` over unscoped enums for new code.
- Avoid template/meta complexity unless the project already uses it.

```cpp
enum class ConnectionState {
    Disconnected,
    Connecting,
    Connected,
    Failed,
};
```

Avoid accidental copies in performance-sensitive paths:

```cpp
void process_packets(std::span<const Packet> packets) {
    for (const Packet& packet : packets) {
        handle(packet);
    }
}
```

Use `string_view` carefully: it does not own data.

```cpp
void log_tag(std::string_view tag); // OK for immediate use
```

Do not store `std::string_view` unless the referenced string lifetime is guaranteed.

---

## Build Systems

### CMake

Prefer modern target-based CMake:

```cmake
add_executable(tool src/main.cpp)
target_compile_features(tool PRIVATE cxx_std_20)
target_include_directories(tool PRIVATE src)
target_link_libraries(tool PRIVATE core)
```

Do not silently change global compiler flags, install paths, output directories, or dependency resolution.

Useful verification:

```bash
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

### Make / Ninja / Meson / Visual Studio

Respect existing workflows. If the repo has a documented build command, use that. Do not replace Make with CMake or Visual Studio with Meson unless explicitly requested.

---

## Game / Engine / Native Plugin Code

For game engines and native plugins:

- Respect engine ownership and lifecycle callbacks.
- Avoid allocations in hot paths if the project is performance-sensitive.
- Do not block the render/game thread with file I/O, network I/O, or long computations.
- Be careful with DLL/shared library boundaries.
- Avoid exceptions crossing C ABI or engine plugin boundaries.
- Match engine math/vector conventions and coordinate system.

Hot path pattern:

```cpp
void update(float dt) {
    // Avoid heap allocations here unless the engine/project accepts them.
    accumulator_ += dt;
    while (accumulator_ >= fixed_step_) {
        simulate(fixed_step_);
        accumulator_ -= fixed_step_;
    }
}
```

---

## Native Interop / ABI

When exposing C/C++ to other languages:

- Prefer a C ABI boundary.
- Avoid exposing STL types across DLL/shared-library boundaries.
- Document allocation/free responsibilities.
- Keep structs versioned if they cross process/plugin boundaries.

```cpp
extern "C" {
    int plugin_init(const PluginConfig* config);
    void plugin_shutdown(void);
}
```

Windows export/import may require project-specific macros:

```cpp
#if defined(_WIN32)
  #define API_EXPORT __declspec(dllexport)
#else
  #define API_EXPORT __attribute__((visibility("default")))
#endif
```

Do not add symbol visibility changes without checking how the project builds shared libraries.

---

## Performance Work

Do not guess. Preserve correctness first, then measure.

Common high-impact checks:

- Avoid accidental copies of large objects.
- Avoid repeated allocations in tight loops.
- Prefer contiguous data for hot iteration.
- Cache expensive lookups when lifetime is clear.
- Avoid virtual/interface dispatch in extremely hot paths only if measured.
- Check algorithmic complexity before micro-optimizing.

```cpp
items.reserve(expected_count);
for (const auto& source : sources) {
    items.emplace_back(source.id, source.value);
}
```

---

## Security-Sensitive Native Code

Use extra caution when touching:

- parsers, binary formats, networking, compression, image/audio decoding
- file paths, archive extraction, plugin loading
- process execution, environment variables, temp files
- crypto, authentication, sandboxing, anti-cheat/anti-tamper logic

Checklist:

```txt
- Validate lengths before reads/writes.
- Check integer overflow before allocation or offset math.
- Avoid TOCTOU file assumptions.
- Normalize and constrain paths before file access.
- Never trust packet/file-declared sizes.
- Keep secrets out of logs and crash dumps.
```

Overflow-safe allocation pattern:

```c
if (count > SIZE_MAX / sizeof(struct item)) {
    return -1;
}
struct item *items = calloc(count, sizeof(struct item));
if (!items) return -1;
```

---

## Testing and Verification

Choose commands from the repo:

```bash
# CMake
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure

# Make
make
make test

# Meson
meson setup build
meson compile -C build
meson test -C build

# Sanitizers when supported
cmake -S . -B build-asan -DCMAKE_BUILD_TYPE=Debug -DCMAKE_CXX_FLAGS="-fsanitize=address,undefined"
cmake --build build-asan
ctest --test-dir build-asan --output-on-failure
```

Do not claim sanitizer coverage if you did not run it. On Windows/MSVC, use the project’s existing test/build scripts when present.

---

## Common AI Mistakes to Avoid

- Treating C and C++ as interchangeable.
- Introducing C++ features into C files.
- Changing headers unnecessarily and causing large rebuilds or ABI breaks.
- Fixing compile errors by deleting code paths instead of understanding build configuration.
- Adding `using namespace std;` in headers.
- Returning pointers/references/views to temporary data.
- Adding `std::move` everywhere and causing moved-from bugs.
- Ignoring Rule of 3/5/0 when adding owning raw pointers.
- Replacing project-specific allocators/logging/asserts with standard ones in engine code.
- Changing CMake globally instead of target-locally.
- Forgetting to add new `.c/.cpp` files to the build target.
- Ignoring platform-specific path, encoding, export, and calling-convention issues.
- Making broad “modernization” changes during a focused bug fix.

---

## Output Standard

When finishing C/C++ work, report:

- What changed and why.
- Public API/header/build changes, if any.
- Ownership/lifetime assumptions touched.
- Build/test commands run, or exact reason they were not run.
- Remaining platform or toolchain risks.
