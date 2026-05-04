---
name: unity
description: Practical Unity guidance for C# gameplay, MonoBehaviour lifecycle, prefabs/scenes, packages, physics, UI, ScriptableObjects, networking, editor tooling, performance, and common agent mistakes. Use only when the task touches Unity project files or Unity runtime/editor behavior.
license: Complete terms in LICENSE.txt
---

# Unity Skill

Use this skill when the repository or task involves Unity projects: `Assets/`, `ProjectSettings/`, `Packages/manifest.json`, `.unity`, `.prefab`, `.asmdef`, `MonoBehaviour`, `ScriptableObject`, Unity C# scripts, editor tools, packages, physics, UI, animation, networking, or Unity build/runtime errors.

Do **not** use it for general C#/.NET work unless Unity engine behavior is part of the task.

## Operating Mode

Before changing Unity code, identify:

- Unity version from `ProjectSettings/ProjectVersion.txt`.
- Render pipeline: Built-in, URP, HDRP.
- Input system: legacy Input Manager or new Input System.
- Project style: MonoBehaviour-heavy, ScriptableObject-driven, ECS/DOTS, custom framework.
- Whether the task affects gameplay, UI, editor tooling, packages, build, assets, or networking.

Unity changes must respect scene/prefab serialization. Do not rename serialized fields casually.

## Unity 80/20 Practices

### MonoBehaviour lifecycle

Use lifecycle methods intentionally:

- `Awake`: internal references and invariant setup.
- `OnEnable`: subscribe to events.
- `Start`: setup that depends on other initialized objects.
- `Update`: input and frame-based presentation.
- `FixedUpdate`: physics forces/movement.
- `OnDisable`: unsubscribe from events.

```csharp
private void OnEnable() => health.Died += HandleDied;
private void OnDisable() => health.Died -= HandleDied;
```

Common mistake: subscribing in `Start()` and never unsubscribing.

### Serialized fields

- Prefer `[SerializeField] private` over public mutable fields.
- Do not rename serialized fields without `[FormerlySerializedAs]` when scenes/prefabs may rely on them.
- Validate required references early.

```csharp
[SerializeField] private Rigidbody rb;

private void Awake()
{
    if (!rb) rb = GetComponent<Rigidbody>();
}
```

### Prefabs and scenes

- Avoid edits that require manual scene wiring unless explicitly noted.
- Do not assume a singleton exists unless the project already uses one.
- Use prefab-safe references and avoid scene-only hard dependencies in reusable components.
- Be careful when changing namespaces, class names, or file names; Unity script references depend on them.

### ScriptableObjects

Use ScriptableObjects for shared immutable-ish design data: items, weapons, enemies, abilities, balance tables.

Do not store per-instance runtime state in shared ScriptableObject assets unless intentionally global.

```csharp
[CreateAssetMenu(menuName = "Game/Weapon Definition")]
public sealed class WeaponDefinition : ScriptableObject
{
    public float Damage;
    public float FireRate;
}
```

### Physics

- Rigidbody movement belongs in `FixedUpdate` or physics callbacks.
- Do not directly set `transform.position` on dynamic rigidbodies every frame unless intentionally teleporting.
- Use collision layers and query masks deliberately.
- For 2D, use `Rigidbody2D`, `Collider2D`, `Physics2D`; do not mix 3D APIs.

### Input

- If the project uses the new Input System, do not introduce legacy `Input.GetKey` patterns unless the project already does.
- Keep input collection separate from movement execution when possible.
- Preserve rebinding/controller support.

### UI

- Identify if the project uses uGUI, UI Toolkit, TextMeshPro, or custom UI.
- Prefer layout groups/anchors over absolute placement.
- Do not instantiate UI elements every frame.
- Use pooling for repeated combat text, inventory rows, or notifications.

### Coroutines, async, and timing

- Coroutines are fine for Unity-timed sequences; cancel them on disable/destroy when needed.
- Avoid `async void` except Unity event handlers.
- Remember Unity APIs generally must run on the main thread.
- `WaitForSeconds` uses scaled time; use `WaitForSecondsRealtime` for pause-resistant timers.

### Packages and dependencies

- Modify `Packages/manifest.json` carefully.
- Do not upgrade major Unity packages casually.
- Avoid adding external packages for small utilities.
- Preserve assembly definitions (`.asmdef`) and references.

### 2D games

High-frequency checks:

- Rigidbody2D vs Rigidbody, Collider2D vs Collider.
- Sprite import settings, pixels-per-unit, compression, filter mode.
- Tilemaps, sorting layers, camera orthographic size.
- Platformer feel: coyote time, jump buffering, variable jump, grounded detection.

### 3D games

High-frequency checks:

- CharacterController vs Rigidbody movement model.
- Camera smoothing and input update order.
- NavMeshAgent usage and avoidance.
- AnimationController state transitions and root motion.
- LOD groups, batching, occlusion, light count, material instancing.

### Networking

- Identify stack: Netcode for GameObjects, Mirror, FishNet, Photon, Steam transport, custom.
- Keep authority explicit.
- Do not trust clients for health, inventory, currency, hit confirmation, or match outcome.
- Avoid sending large object graphs every tick.
- Separate prediction/interpolation from authoritative state.

### Performance

- Avoid repeated `FindObjectOfType`, `GameObject.Find`, `GetComponent` in hot paths; cache references.
- Pool frequent projectiles/effects/UI rows.
- Avoid LINQ in hot per-frame code when allocations matter.
- Use Profiler-driven optimization, not blind rewrites.

## Common AI Mistakes

- Inventing Unity APIs or using wrong namespace/package.
- Mixing 2D and 3D physics APIs.
- Renaming serialized fields/classes and breaking prefab references.
- Adding public mutable fields everywhere.
- Using `Update()` for physics movement.
- Forgetting event unsubscription.
- Storing runtime state in ScriptableObject assets.
- Ignoring assembly definitions and package versions.
- Trusting clients in multiplayer code.
- Adding dependencies instead of using Unity-native features already present.

## Verification

Prefer existing project commands. In CI or batch mode, common patterns are:

```bash
Unity -batchmode -quit -projectPath . -runTests -testPlatform EditMode
Unity -batchmode -quit -projectPath . -runTests -testPlatform PlayMode
```

Also inspect:

```txt
ProjectSettings/ProjectVersion.txt
Packages/manifest.json
Packages/packages-lock.json
```

When Unity cannot be run, state that scene/prefab serialization and editor compilation were not verified.
