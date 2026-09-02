#!/usr/bin/env python3
from pathlib import Path


def patch(path: str, replacements):
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    for label, old, new in replacements:
        count = text.count(old)
        if count != 1:
            raise SystemExit(f"{path}: {label}: expected exactly one source snippet, got {count}")
        text = text.replace(old, new, 1)
    p.write_text(text, encoding="utf-8")


patch(
    "agent/NXGO.Agent.Core/HandleRegistry.cs",
    [(
        "owned resolve insertion",
        '''    public bool Release(ObjectHandleToken token)\n    {\n''',
        '''    /// <summary>\n    /// Resolves a child only when its canonical registry owner matches the\n    /// supplied live owner handle. This lets hosts validate cross-object\n    /// relationships without touching native NX objects on transport threads.\n    /// </summary>\n    public T ResolveOwned(ObjectHandleToken token, ObjectHandleToken owner, params string[] expectedKinds)\n    {\n        if (token == null) throw new StaleObjectHandleException("object handle is null");\n        if (owner == null) throw new StaleObjectHandleException("owner handle is null");\n\n        lock (_sync)\n        {\n            ValidateIdentityLocked(owner);\n            ValidateIdentityLocked(token);\n            var entry = _entries[token.ObjectId];\n            if (!string.Equals(entry.OwnerObjectId, owner.ObjectId, StringComparison.Ordinal))\n            {\n                throw new StaleObjectHandleException(\n                    $"object {token.ObjectId} is not owned by {owner.ObjectId}");\n            }\n            if (expectedKinds != null && expectedKinds.Length > 0 &&\n                !expectedKinds.Any(k => string.Equals(k, entry.Token.Kind, StringComparison.OrdinalIgnoreCase)))\n            {\n                throw new StaleObjectHandleException(\n                    $"wrong object kind for {token.ObjectId}: got {entry.Token.Kind}, expected one of [{string.Join(", ", expectedKinds)}]");\n            }\n            return entry.Target;\n        }\n    }\n\n    public bool Release(ObjectHandleToken token)\n    {\n''',
    )],
)

patch(
    "agent/NXGO.Agent.Core.Tests/HandleRegistryTests.cs",
    [(
        "ownership validation test",
        '''    [Fact]\n    public void Missing_generation_cannot_release_a_live_object()\n''',
        '''    [Fact]\n    public void Resolve_owned_rejects_cross_owner_handles_without_touching_target()\n    {\n        var registry = new HandleRegistry<object>("session-a", 6, capacity: 8);\n        var ownerA = registry.Register(new object(), "Part");\n        var ownerB = registry.Register(new object(), "Part");\n        var target = new object();\n        var component = registry.Register(target, "Component", ownerObjectId: ownerA.ObjectId);\n\n        Assert.Same(target, registry.ResolveOwned(component, ownerA, "Component"));\n        Assert.Throws<StaleObjectHandleException>(() =>\n            registry.ResolveOwned(component, ownerB, "Component"));\n        Assert.NotNull(registry.Resolve(component, "Component"));\n    }\n\n    [Fact]\n    public void Missing_generation_cannot_release_a_live_object()\n''',
    )],
)

patch(
    "agent/NXGO.Agent.NXHost/EntryPoint.Assembly.cs",
    [
        (
            "add component defer part resolve",
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n        var part = (Part)Registry.Resolve(partHandle, "Part");\n        var partPath = GetString(payload, "part_path", string.Empty);\n''',
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n        var partPath = GetString(payload, "part_path", string.Empty);\n''',
        ),
        (
            "add component execution resolve",
            '''        {\n            Health.RequireReusable();\n            Journal.MarkStarted(requestId);\n\n            PartLoadStatus loadStatus;\n''',
            '''        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(partHandle, "Part");\n            Journal.MarkStarted(requestId);\n\n            PartLoadStatus loadStatus;\n''',
        ),
        (
            "remove component transport native access",
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n        var componentHandle = RequireHandle(payload, "component_ref", "Component");\n        var part = (Part)Registry.Resolve(partHandle, "Part");\n        var component = (Component)Registry.Resolve(componentHandle, "Component");\n        if (component.OwningPart != null && component.OwningPart != part)\n        {\n            throw new StaleObjectHandleException("component handle does not belong to the supplied assembly part");\n        }\n\n        return MapMutation(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            Journal.MarkStarted(requestId);\n            part.ComponentAssembly.RemoveComponent(component);\n''',
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n        var componentHandle = RequireHandle(payload, "component_ref", "Component");\n\n        return MapMutation(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(partHandle, "Part");\n            var component = (Component)Registry.ResolveOwned(componentHandle, partHandle, "Component");\n            Journal.MarkStarted(requestId);\n            part.ComponentAssembly.RemoveComponent(component);\n''',
        ),
        (
            "tree defer part resolve",
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n        var part = (Part)Registry.Resolve(partHandle, "Part");\n\n        return MapRead(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var remaining = MaxAssemblySnapshotNodes;\n''',
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n\n        return MapRead(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(partHandle, "Part");\n            var remaining = MaxAssemblySnapshotNodes;\n''',
        ),
        (
            "bom defer part resolve",
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n        var part = (Part)Registry.Resolve(partHandle, "Part");\n\n        return MapRead(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var groups = new Dictionary<string, List<string>>(StringComparer.OrdinalIgnoreCase);\n''',
            '''        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");\n\n        return MapRead(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(partHandle, "Part");\n            var groups = new Dictionary<string, List<string>>(StringComparer.OrdinalIgnoreCase);\n''',
        ),
        (
            "snapshot depth predictable error",
            '''            throw new InvalidOperationException("assembly snapshot depth exceeds canonical safety limit");\n''',
            '''            throw new ArgumentException("assembly snapshot depth exceeds canonical safety limit");\n''',
        ),
        (
            "snapshot count predictable error",
            '''            throw new InvalidOperationException("assembly snapshot node count exceeds canonical safety limit");\n''',
            '''            throw new ArgumentException("assembly snapshot node count exceeds canonical safety limit");\n''',
        ),
    ],
)

patch(
    "agent/NXGO.Agent.NXHost/EntryPoint.cs",
    [
        (
            "assembly routing",
            '''                case "transaction.rollback":\n                    return StartTransactionRollback(session, executor, requestId, requestPayload, token);\n\n                default:\n''',
            '''                case "transaction.rollback":\n                    return StartTransactionRollback(session, executor, requestId, requestPayload, token);\n                case "assembly.add_component":\n                    return StartAssemblyAddComponent(executor, requestId, requestPayload, token);\n                case "assembly.query_tree":\n                    return StartAssemblyQueryTree(executor, requestId, requestPayload, token);\n                case "assembly.query_bom":\n                    return StartAssemblyQueryBOM(executor, requestId, requestPayload, token);\n                case "assembly.remove_component":\n                    return StartAssemblyRemoveComponent(executor, requestId, requestPayload, token);\n\n                default:\n''',
        ),
        (
            "part save temporal resolve",
            '''        var handle = RequireHandle(payload, "part_ref", "Part");\n        var part = (Part)Registry.Resolve(handle, "Part");\n\n        return MapMutation(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            Journal.MarkStarted(requestId);\n            part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);\n''',
            '''        var handle = RequireHandle(payload, "part_ref", "Part");\n\n        return MapMutation(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(handle, "Part");\n            Journal.MarkStarted(requestId);\n            part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);\n''',
        ),
        (
            "part close temporal resolve",
            '''        var handle = RequireHandle(payload, "part_ref", "Part");\n        var part = (Part)Registry.Resolve(handle, "Part");\n        var save = GetBool(payload, "save", false);\n\n        return MapMutation(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            Journal.MarkStarted(requestId);\n            var name = part.Name;\n''',
            '''        var handle = RequireHandle(payload, "part_ref", "Part");\n        var save = GetBool(payload, "save", false);\n\n        return MapMutation(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(handle, "Part");\n            Journal.MarkStarted(requestId);\n            var name = part.Name;\n''',
        ),
        (
            "part summary temporal resolve",
            '''        var handle = RequireHandle(payload, "part_ref", "Part");\n        var part = (Part)Registry.Resolve(handle, "Part");\n\n        return MapRead(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var bodyCount = 0;\n''',
            '''        var handle = RequireHandle(payload, "part_ref", "Part");\n\n        return MapRead(requestId, executor.EnqueueTracked(() =>\n        {\n            Health.RequireReusable();\n            var part = (Part)Registry.Resolve(handle, "Part");\n            var bodyCount = 0;\n''',
        ),
        (
            "read predictable error mapping",
            '''        catch (TaskCanceledException ex)\n        {\n            return FormatError(requestId, "CANCELLED", ex.Message, true);\n        }\n        catch (Exception ex)\n''',
            '''        catch (TaskCanceledException ex)\n        {\n            return FormatError(requestId, "CANCELLED", ex.Message, true);\n        }\n        catch (ArgumentException ex)\n        {\n            return FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);\n        }\n        catch (HandleRegistryCapacityException ex)\n        {\n            return FormatError(requestId, "CAPACITY", ex.Message, true);\n        }\n        catch (HandleScopeCapacityException ex)\n        {\n            return FormatError(requestId, "CAPACITY", ex.Message, true);\n        }\n        catch (StaleObjectHandleException ex)\n        {\n            return FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);\n        }\n        catch (Exception ex)\n''',
        ),
        (
            "assembly journal classification",
            '''            case "transaction.begin":\n            case "transaction.commit":\n            case "transaction.rollback":\n                return true;\n''',
            '''            case "transaction.begin":\n            case "transaction.commit":\n            case "transaction.rollback":\n            case "assembly.add_component":\n            case "assembly.remove_component":\n                return true;\n''',
        ),
        (
            "assembly capabilities",
            '''                "transaction.begin",\n                "transaction.commit",\n                "transaction.rollback",\n                "shutdown",\n''',
            '''                "transaction.begin",\n                "transaction.commit",\n                "transaction.rollback",\n                "assembly.add_component",\n                "assembly.query_tree",\n                "assembly.query_bom",\n                "assembly.remove_component",\n                "shutdown",\n''',
        ),
    ],
)

print("canonical temporal-handle and Assembly wiring applied")
