---
name: godot
description: Practical Godot engine guidance for GDScript/C# projects, 2D/3D games, scenes/nodes/resources, physics, UI, multiplayer, exports, and common agent mistakes. Use only when the task touches Godot files or Godot engine behavior.
license: Complete terms in LICENSE.txt
---

# Godot Skill

Use this skill when the repository or task involves `project.godot`, `.gd`, `.tscn`, `.tres`, `.res`, Godot C# scripts, scenes, nodes, resources, signals, exports, physics, multiplayer, or Godot build/runtime errors.

Do **not** use it for generic game design without Godot implementation, or for Unity/Unreal projects.

## Operating Mode

Before editing, identify:

- Godot version, especially 3.x vs 4.x.
- Language: GDScript, C#, or mixed.
- Main scenes, autoloads/singletons, and scene ownership.
- Whether the task is runtime gameplay, UI, networking, tools/editor, export, or asset pipeline.
- Existing patterns for signals, groups, resources, input maps, and node paths.

Prefer small, scene-compatible changes. Do not rewrite project structure unless requested.

## Godot 80/20 Practices

### Scene tree and node design

- Favor composition with nodes/components over giant scripts.
- Keep responsibilities clear: player controller, weapon, camera, UI, network sync, save system, etc.
- Avoid hardcoding fragile absolute node paths when `$Child` or exported `NodePath` references are more stable.
- Use groups for cross-scene discovery when objects are dynamic.
- Avoid scanning the whole tree every frame.

```gdscript
@export var target_path: NodePath
@onready var target: Node = get_node_or_null(target_path)
```

### GDScript correctness

- Respect Godot 4 syntax: `@export`, `@onready`, `Callable`, typed signals, `CharacterBody2D/3D`.
- Do not mix Godot 3 APIs like `KinematicBody2D` into Godot 4 projects.
- Prefer typed variables/signatures for gameplay-critical code.
- Avoid implicit globals and magic string lookups when constants help.

```gdscript
signal health_changed(current: int, max_value: int)

var health: int = 100

func apply_damage(amount: int) -> void:
    health = max(health - amount, 0)
    health_changed.emit(health, 100)
```

### Lifecycle

- Use `_ready()` for node references and setup.
- Use `_process(delta)` for visual/non-physics updates.
- Use `_physics_process(delta)` for movement, physics, and deterministic gameplay.
- Do not put expensive pathfinding, file IO, or scene searches inside per-frame loops.

### Movement and physics

For Godot 4, use `CharacterBody2D/3D` movement patterns and preserve existing collision assumptions.

```gdscript
func _physics_process(delta: float) -> void:
    var axis := Input.get_axis("move_left", "move_right")
    velocity.x = axis * speed
    move_and_slide()
```

Common mistakes:

- Using render delta for physics movement.
- Directly setting global position on physics bodies every frame.
- Forgetting to handle floor/wall states for platformers.
- Replacing `move_and_slide()` behavior with teleport-style movement.

### Input

- Use Input Map actions, not raw keycodes, unless implementing a keybinding screen.
- Preserve controller support when it exists.
- Avoid processing gameplay input in UI nodes unless the UI owns that interaction.

### Signals and events

- Use signals for decoupling UI, gameplay, and state changes.
- Avoid long chains of `get_parent().get_parent()`.
- Avoid duplicate signal connections.

### Resources and data

- Use `.tres`/custom `Resource` classes for reusable data: weapons, abilities, enemies, inventory items, levels.
- Do not mutate shared resources at runtime unless intentionally global; duplicate when per-instance state is needed.

```gdscript
var runtime_stats := base_stats.duplicate(true)
```

### UI / Control nodes

- Use anchors/containers instead of absolute positions for responsive UI.
- Keep gameplay state separate from UI rendering.
- For premium-feeling UI, coordinate typography, spacing, animation, and input feedback; do not just add gradients.

### 2D games

High-frequency checks:

- Pixel art: import filtering, snap settings, camera smoothing, viewport stretch mode.
- Platformer: coyote time, jump buffering, variable jump height, floor detection.
- Top-down: normalized diagonal movement, collision layers/masks, nav regions.
- Effects: particles, screen shake, hitstop, tweened UI feedback.

### 3D games

High-frequency checks:

- Separate camera controller from player movement when possible.
- Keep physics, camera smoothing, and animation updates in the correct loop.
- Use collision layers/masks intentionally.
- Optimize with LOD, occlusion, batching/import settings, and avoiding excessive dynamic lights.
- Avoid spawning heavy scenes every frame; pool frequent projectiles/effects.

### Multiplayer

- Identify if the project uses high-level multiplayer, ENet/WebRTC, Steam, or a custom transport.
- Keep authority explicit: server-authoritative, host-authoritative, or peer-authoritative.
- Never trust client-side damage, inventory, currency, or match results.
- Sync state deliberately; do not replicate entire nodes by accident.
- Use RPC annotations carefully in Godot 4.

```gdscript
@rpc("authority", "call_remote", "reliable")
func apply_server_state(state: Dictionary) -> void:
    # Validate shape before applying.
    pass
```

### Performance

- Avoid per-frame allocations in hot gameplay loops when easy.
- Cache node references.
- Use object pools for bullets, damage numbers, particles, and repeated effects.
- Profile before large rewrites.
- Check import settings before blaming script performance.

### Export/build

- Preserve export presets and target platform assumptions.
- Check case-sensitive paths for Linux exports.
- Avoid editor-only APIs in exported runtime code.
- For C#, check `.csproj`, Godot .NET version, and generated bindings.

## Common AI Mistakes

- Mixing Godot 3 and Godot 4 APIs.
- Inventing nonexistent methods or signal names.
- Editing `.tscn` manually without preserving scene format.
- Breaking exported variables used by scenes.
- Moving logic out of nodes without updating scene references.
- Using absolute paths that break when scenes are instanced.
- Treating resources as instance-local when they are shared assets.
- Trusting multiplayer clients for authoritative game state.
- Adding heavy logic to `_process()` instead of event-driven code.
- Ignoring collision layers/masks and input map names already present.

## Verification

Prefer project-native checks:

```bash
godot --version
godot --headless --check-only --path .
godot --headless --path . --quit-after 1
```

For C# projects, also check build/test commands if configured:

```bash
dotnet build
dotnet test
```

When unable to run Godot, inspect scenes/scripts for version-specific APIs and state what was not verified.
