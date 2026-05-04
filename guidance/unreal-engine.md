---
name: unreal-engine
description: Practical Unreal Engine guidance for C++/Blueprint projects, actors/components, reflection macros, modules, replication, input, UI, assets, build files, performance, and common agent mistakes. Use only when the task touches Unreal project files or Unreal engine behavior.
license: Complete terms in LICENSE.txt
---

# Unreal Engine Skill

Use this skill when the repository or task involves Unreal Engine projects: `.uproject`, `.uplugin`, `Source/`, `.Build.cs`, `.Target.cs`, `UCLASS`, `USTRUCT`, `UPROPERTY`, `UFUNCTION`, Blueprints, Actors, Components, replication, Gameplay Ability System, Slate/UMG, packaging, or Unreal build/runtime/editor errors.

Do **not** use it for generic C++ unless Unreal engine APIs or project structure are involved.

## Operating Mode

Before editing, identify:

- Unreal version and project type.
- C++ vs Blueprint-heavy architecture.
- Modules/plugins and relevant `.Build.cs` dependencies.
- Target area: gameplay, UI, editor tool, networking, assets, build/package, performance.
- Existing naming conventions and class hierarchy.

Unreal work must respect reflection, asset references, module boundaries, and Blueprint compatibility.

## Unreal 80/20 Practices

### Class architecture

- Use Actors for world entities and ActorComponents for reusable behavior.
- Keep gameplay logic out of monolithic pawns/controllers when a component is cleaner.
- Respect Unreal prefixes: `A` Actor, `U` UObject, `F` struct, `I` interface, `E` enum.
- Avoid changing reflected class/member names casually; Blueprints/assets may depend on them.

### Reflection macros

Use Unreal reflection correctly when exposing data/functions to editor, serialization, Blueprint, networking, or GC.

```cpp
UCLASS()
class MYGAME_API AMyPickup : public AActor
{
    GENERATED_BODY()

public:
    UPROPERTY(EditAnywhere, BlueprintReadOnly, Category="Pickup")
    int32 Value = 1;

    UFUNCTION(BlueprintCallable, Category="Pickup")
    void Collect(APawn* Collector);
};
```

Common mistake: adding raw UObject pointers without `UPROPERTY`, causing GC/reference issues.

### Memory and ownership

- Prefer Unreal containers/types where appropriate: `TArray`, `TMap`, `TSet`, `FString`, `FName`, `TObjectPtr`.
- Use `UPROPERTY` for UObject references that must survive garbage collection.
- Avoid raw `new/delete` for UObjects; use engine creation APIs.
- Use `TWeakObjectPtr` for non-owning object references.

### Modules and build files

- Add dependencies to `.Build.cs` explicitly.
- Do not include private engine headers unless necessary.
- Keep public/private include boundaries clean.
- Fix compile errors by adding the correct module dependency, not by dumping includes everywhere.

```csharp
PublicDependencyModuleNames.AddRange(new[] { "Core", "CoreUObject", "Engine" });
PrivateDependencyModuleNames.AddRange(new[] { "UMG" });
```

### Gameplay framework

Know the standard roles:

- `GameMode`: server-only rules.
- `GameState`: replicated match state.
- `PlayerController`: player input/ownership bridge.
- `PlayerState`: replicated per-player state.
- `Pawn/Character`: possessed world entity.
- `ActorComponent`: reusable behavior.

Do not put client-needed state only in `GameMode`.

### Networking / replication

- Keep server authority explicit.
- Never trust clients for damage, inventory, currency, objectives, or match outcome.
- Use RPCs deliberately: Server, Client, NetMulticast.
- Replicate minimal state; avoid large per-tick payloads.
- Use `OnRep` for client-side reactions to replicated state.

```cpp
UPROPERTY(ReplicatedUsing=OnRep_Health)
float Health = 100.f;

UFUNCTION()
void OnRep_Health();
```

Remember to implement `GetLifetimeReplicatedProps`.

### Input

- Identify whether project uses legacy input or Enhanced Input.
- Do not introduce a second input stack casually.
- Keep mapping contexts and actions consistent.

### UI

- UMG for most game UI; Slate for lower-level/editor/custom widgets.
- Keep UI presentation separate from authoritative gameplay state.
- Avoid polling expensive game state every tick in widgets when events/delegates can update UI.

### Blueprints and assets

- C++ changes can break Blueprint assets if reflected names/types change.
- Prefer adding compatible properties/functions instead of renaming/removing used ones.
- Use BlueprintImplementableEvent/BlueprintNativeEvent intentionally.
- Do not assume asset paths are stable; use soft references for optional/streamed assets.

### Performance

- Avoid heavy logic in `Tick` unless necessary; disable ticking by default where possible.
- Use timers/events for periodic work.
- Profile before optimizing: Unreal Insights, stat commands, profiler captures.
- Use object pooling for frequent transient actors if spawn/despawn cost matters.
- Be careful with dynamic material instances, traces, and replicated actor counts.

### Packaging

- Distinguish editor-only code from runtime code.
- Keep plugin/module settings platform-aware.
- Watch case-sensitive paths for Linux builds.
- Avoid editor module dependencies in runtime modules.

## Common AI Mistakes

- Treating Unreal C++ like normal C++ and ignoring reflection/GC.
- Forgetting `GENERATED_BODY()` or required includes.
- Adding UObject raw pointers without `UPROPERTY`.
- Renaming reflected fields and breaking Blueprints.
- Putting replicated/client-needed state in `GameMode`.
- Forgetting `GetLifetimeReplicatedProps`.
- Calling server-only logic on clients.
- Enabling expensive `Tick` everywhere.
- Adding dependencies to code but not `.Build.cs`.
- Mixing Enhanced Input and legacy input without checking the project.

## Verification

Inspect first:

```txt
*.uproject
Source/*/*.Build.cs
Source/*/*.Target.cs
Config/*.ini
Plugins/*/*.uplugin
```

Prefer the repository's documented Unreal build scripts when present. When Unreal cannot be built locally, state that reflection, Blueprint compatibility, and packaging were not verified.
