using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;
using NXOpen.Assemblies;
using NXOpen.Positioning;
using NXOpen.UF;

namespace NXGO.Agent.NXHost;

public static partial class EntryPoint
{
    private const int MaxAssemblySnapshotNodes = 16384;
    private const int MaxAssemblySnapshotDepth = 64;

    private static Task<byte[]> StartAssemblyAddComponent(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var partPath = GetString(payload, "part_path", string.Empty);
        if (string.IsNullOrWhiteSpace(partPath))
        {
            throw new ArgumentException("part_path is required for assembly.add_component");
        }

        var componentName = GetString(payload, "component_name", string.Empty);
        if (string.IsNullOrWhiteSpace(componentName))
        {
            componentName = "comp_" + Guid.NewGuid().ToString("N").Substring(0, 6);
        }
        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var orientation = GetDoubleArray(payload, "orientation", 9, new[]
        {
            1.0, 0.0, 0.0,
            0.0, 1.0, 0.0,
            0.0, 0.0, 1.0,
        });
        var layer = GetInt(payload, "layer", 1);
        if (layer < 1 || layer > 256)
        {
            throw new ArgumentException("assembly component layer must be in range 1..256");
        }

        var matrix = new Matrix3x3
        {
            Xx = orientation[0], Xy = orientation[1], Xz = orientation[2],
            Yx = orientation[3], Yy = orientation[4], Yz = orientation[5],
            Zx = orientation[6], Zy = orientation[7], Zz = orientation[8],
        };

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            PartLoadStatus loadStatus;
            var component = part.ComponentAssembly.AddComponent(
                partPath,
                "MODEL",
                componentName,
                new Point3d(origin[0], origin[1], origin[2]),
                matrix,
                layer,
                out loadStatus);
            if (loadStatus != null) loadStatus.Dispose();

            var handle = Registry.Register(
                component,
                "Component",
                ownerObjectId: partHandle.ObjectId);
            uint nativeTag = 0;
            try { nativeTag = (uint)component.Tag; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["component_ref"] = FormatHandle(handle, component),
                ["component_name"] = component.DisplayName ?? component.Name ?? componentName,
                ["part_path"] = partPath.Replace('\\', '/'),
                ["native_tag"] = nativeTag,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyRemoveComponent(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var componentHandle = RequireHandle(payload, "component_ref", "Component");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var component = (Component)Registry.ResolveOwned(componentHandle, partHandle, "Component");
            Journal.MarkStarted(requestId);
            part.ComponentAssembly.RemoveComponent(component);
            Registry.ReleaseWithDependents(componentHandle);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["removed"] = true,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryTree(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var remaining = MaxAssemblySnapshotNodes;
            var root = part.ComponentAssembly != null ? part.ComponentAssembly.RootComponent : null;
            if (root == null) throw new InvalidOperationException("assembly root component is unavailable");
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["root"] = SerializeComponentSnapshot(root, 0, ref remaining),
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryBOM(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var groups = new Dictionary<string, List<string>>(StringComparer.OrdinalIgnoreCase);
            var remaining = MaxAssemblySnapshotNodes;
            var root = part.ComponentAssembly != null ? part.ComponentAssembly.RootComponent : null;
            if (root == null) throw new InvalidOperationException("assembly root component is unavailable");
            CollectBOMSnapshot(root, groups, 0, ref remaining);

            var keys = new List<string>(groups.Keys);
            keys.Sort(StringComparer.OrdinalIgnoreCase);
            var items = new List<object>(keys.Count);
            foreach (var path in keys)
            {
                var names = groups[path];
                names.Sort(StringComparer.OrdinalIgnoreCase);
                items.Add(new Dictionary<string, object>
                {
                    ["part_name"] = Path.GetFileName(path),
                    ["part_path"] = path.Replace('\\', '/'),
                    ["quantity"] = names.Count,
                    ["component_names"] = names,
                });
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["items"] = items,
            });
        }, token));
    }

    // Tree/BOM are snapshots, not object-identity APIs. Unlike the legacy
    // Agent, the canonical adapter deliberately does not allocate registry
    // handles while traversing read-only assembly data.
    private static Dictionary<string, object> SerializeComponentSnapshot(Component component, int depth, ref int remaining)
    {
        if (component == null)
        {
            return new Dictionary<string, object>
            {
                ["component_ref"] = new Dictionary<string, object>(),
                ["name"] = string.Empty,
                ["display_name"] = string.Empty,
                ["prototype_path"] = string.Empty,
                ["position"] = new[] { 0.0, 0.0, 0.0 },
                ["children"] = new List<object>(),
            };
        }
        RequireAssemblySnapshotBudget(depth, ref remaining);

        Point3d position;
        Matrix3x3 orientation;
        try { component.GetPosition(out position, out orientation); }
        catch { position = new Point3d(0.0, 0.0, 0.0); }

        var children = new List<object>();
        Component[] childArray;
        try { childArray = component.GetChildren() ?? new Component[0]; }
        catch { childArray = new Component[0]; }
        foreach (var child in childArray)
        {
            children.Add(SerializeComponentSnapshot(child, depth + 1, ref remaining));
        }

        return new Dictionary<string, object>
        {
            ["component_ref"] = new Dictionary<string, object>(),
            ["name"] = component.Name ?? string.Empty,
            ["display_name"] = component.DisplayName ?? string.Empty,
            ["prototype_path"] = ComponentPrototypePath(component),
            ["position"] = new[] { position.X, position.Y, position.Z },
            ["children"] = children,
        };
    }

    private static void CollectBOMSnapshot(
        Component component,
        Dictionary<string, List<string>> groups,
        int depth,
        ref int remaining)
    {
        if (component == null) return;
        RequireAssemblySnapshotBudget(depth, ref remaining);

        Component[] children;
        try { children = component.GetChildren() ?? new Component[0]; }
        catch { children = new Component[0]; }
        foreach (var child in children)
        {
            var path = ComponentPrototypePath(child);
            if (string.IsNullOrWhiteSpace(path))
            {
                path = "unresolved/" + (child.DisplayName ?? child.Name ?? "component");
            }
            List<string>? names;
            if (!groups.TryGetValue(path, out names))
            {
                names = new List<string>();
                groups.Add(path, names);
            }
            if (!groups.TryGetValue(path, out names))
            {
                names = new List<string>();
                groups.Add(path, names);
            }
            names.Add(child.DisplayName ?? child.Name ?? string.Empty);
            CollectBOMSnapshot(child, groups, depth + 1, ref remaining);
        }
    }

    private static string ComponentPrototypePath(Component component)
    {
        try
        {
            if (component.Prototype != null && component.Prototype.OwningPart != null)
            {
                return (component.Prototype.OwningPart.FullPath ?? string.Empty).Replace('\\', '/');
            }
        }
        catch { }
        return string.Empty;
    }

    private static void RequireAssemblySnapshotBudget(int depth, ref int remaining)
    {
        if (depth > MaxAssemblySnapshotDepth)
        {
            throw new ArgumentException("assembly snapshot depth exceeds canonical safety limit");
        }
        if (remaining <= 0)
        {
            throw new ArgumentException("assembly snapshot node count exceeds canonical safety limit");
        }
        remaining--;
    }

    private static int GetInt(Dictionary<string, object> source, string key, int defaultValue)
    {
        object? value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        return Convert.ToInt32(value, CultureInfo.InvariantCulture);
    }

    private static Task<byte[]> StartAssemblyCreateConstraint(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var typeStr = GetString(payload, "type", string.Empty);
        if (string.IsNullOrWhiteSpace(typeStr))
        {
            throw new ArgumentException("type is required for assembly.create_constraint");
        }
        var target1Handle = RequireAnyHandle(payload, "target_ref_1");
        var target2Handle = TryGetHandle(payload, "target_ref_2");
        var alignStr = GetString(payload, "alignment", "infer_align");
        var offset = GetDouble(payload, "offset", 0.0);
        var name = GetString(payload, "name", string.Empty);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            if (part.ComponentAssembly == null)
            {
                throw new InvalidOperationException("part has no component assembly");
            }

            var positioner = part.ComponentAssembly.Positioner;
            positioner.ClearNetwork();
            var network = (ComponentNetwork)positioner.EstablishNetwork();
            var constraint = (ComponentConstraint)positioner.CreateConstraint(true);

            switch (typeStr.ToLowerInvariant())
            {
                case "touch":
                    constraint.ConstraintType = Constraint.Type.Touch;
                    break;
                case "concentric":
                    constraint.ConstraintType = Constraint.Type.Concentric;
                    break;
                case "fix":
                    constraint.ConstraintType = Constraint.Type.Fix;
                    break;
                case "distance":
                    constraint.ConstraintType = Constraint.Type.Distance;
                    break;
                case "parallel":
                    constraint.ConstraintType = Constraint.Type.Parallel;
                    break;
                case "perpendicular":
                    constraint.ConstraintType = Constraint.Type.Perpendicular;
                    break;
                case "center12":
                    constraint.ConstraintType = Constraint.Type.Center12;
                    break;
                case "center22":
                    constraint.ConstraintType = Constraint.Type.Center22;
                    break;
                case "angle":
                    constraint.ConstraintType = Constraint.Type.Angle;
                    break;
                case "fit":
                    constraint.ConstraintType = Constraint.Type.Fit;
                    break;
                case "bond":
                    constraint.ConstraintType = Constraint.Type.Bond;
                    break;
                case "align_lock":
                    constraint.ConstraintType = Constraint.Type.AlignLock;
                    break;
                default:
                    throw new ArgumentException("unsupported constraint type: " + typeStr);
            }

            switch (alignStr.ToLowerInvariant())
            {
                case "co_align":
                    constraint.ConstraintAlignment = Constraint.Alignment.CoAlign;
                    break;
                case "contra_align":
                    constraint.ConstraintAlignment = Constraint.Alignment.ContraAlign;
                    break;
                case "infer_align":
                default:
                    constraint.ConstraintAlignment = Constraint.Alignment.InferAlign;
                    break;
            }

            var raw1 = Registry.Resolve(target1Handle);
            var obj1 = (NXObject)raw1;
            NXObject movable1 = (raw1 is Component c1) ? c1 : ((obj1 is DisplayableObject d1 && d1.OwningComponent != null) ? d1.OwningComponent : obj1);
            constraint.CreateConstraintReference(movable1, obj1, false, false);

            if (target2Handle != null)
            {
                var raw2 = Registry.Resolve(target2Handle);
                var obj2 = (NXObject)raw2;
                NXObject movable2 = (raw2 is Component c2) ? c2 : ((obj2 is DisplayableObject d2 && d2.OwningComponent != null) ? d2.OwningComponent : obj2);
                constraint.CreateConstraintReference(movable2, obj2, false, false);
            }

            if (Math.Abs(offset) > 1e-9)
            {
                try { constraint.OffsetRightHandSide = offset.ToString(CultureInfo.InvariantCulture); } catch { }
            }

            if (!string.IsNullOrWhiteSpace(name))
            {
                try { constraint.SetName(name); } catch { }
            }

            network.AddConstraint(constraint);
            try
            {
                network.Solve();
                network.ApplyToModel();
            }
            finally
            {
                positioner.ClearNetwork();
            }

            var cHandle = Registry.Register(constraint, "ComponentConstraint", ownerObjectId: partHandle.ObjectId);
            uint nativeTag = 0;
            try { nativeTag = (uint)constraint.Tag; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["constraint_ref"] = FormatHandle(cHandle, constraint),
                ["name"] = constraint.Name ?? constraint.JournalIdentifier ?? (string.IsNullOrWhiteSpace(name) ? "Constraint" : name),
                ["type"] = constraint.ConstraintType.ToString().ToLowerInvariant(),
                ["alignment"] = constraint.ConstraintAlignment.ToString().ToLowerInvariant(),
                ["status"] = constraint.GetConstraintStatus().ToString(),
                ["native_tag"] = nativeTag,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryConstraints(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var positioner = part.ComponentAssembly != null ? part.ComponentAssembly.Positioner : null;
            var list = new List<object>();
            if (positioner != null)
            {
                foreach (Constraint c in positioner.Constraints)
                {
                    var cHandle = Registry.Register(c, "ComponentConstraint", ownerObjectId: partHandle.ObjectId);
                    uint tag = 0;
                    try { tag = (uint)c.Tag; } catch { }
                    list.Add(new Dictionary<string, object>
                    {
                        ["constraint_ref"] = FormatHandle(cHandle, c),
                        ["name"] = c.Name ?? c.JournalIdentifier ?? "Constraint",
                        ["type"] = c.ConstraintType.ToString().ToLowerInvariant(),
                        ["alignment"] = c.ConstraintAlignment.ToString().ToLowerInvariant(),
                        ["status"] = c.GetConstraintStatus().ToString(),
                        ["suppressed"] = c.Suppressed,
                        ["native_tag"] = tag,
                    });
                }
            }
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["constraints"] = list,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyDeleteConstraint(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var constraintHandle = RequireHandle(payload, "constraint_ref", "ComponentConstraint");
        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var constraint = (Constraint)Registry.Resolve(constraintHandle, "ComponentConstraint");
            Journal.MarkStarted(requestId);
            session.UpdateManager.AddToDeleteList(constraint);
            session.UpdateManager.DoUpdate(0);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["deleted"] = true,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblySetConstraintSuppressed(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var constraintHandle = RequireHandle(payload, "constraint_ref", "ComponentConstraint");
        var suppressed = GetBool(payload, "suppressed", false);
        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var constraint = (Constraint)Registry.Resolve(constraintHandle, "ComponentConstraint");
            Journal.MarkStarted(requestId);
            constraint.Suppressed = suppressed;
            session.UpdateManager.DoUpdate(0);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["suppressed"] = constraint.Suppressed,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyCreateArrangement(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var name = GetString(payload, "name", string.Empty);
        if (string.IsNullOrWhiteSpace(name)) throw new ArgumentException("name is required for assembly.create_arrangement");
        var baseHandle = TryGetHandle(payload, "base_arrangement_ref");
        var isolated = GetBool(payload, "isolated", false);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");

            Arrangement? baseArr = null;
            if (baseHandle != null)
            {
                baseArr = (Arrangement)Registry.Resolve(baseHandle, "Arrangement");
            }
            else
            {
                baseArr = part.ComponentAssembly.ActiveArrangement;
            }

            var collection = part.ComponentAssembly.Arrangements;
            Arrangement created = isolated
                ? collection.CreateIsolated(baseArr, name)
                : collection.Create(baseArr, name);

            var aHandle = Registry.Register(created, "Arrangement", ownerObjectId: partHandle.ObjectId);
            uint tag = 0;
            try { tag = (uint)created.Tag; } catch { }

            bool isActive = false;
            try { isActive = part.ComponentAssembly.ActiveArrangement == created; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["arrangement_ref"] = FormatHandle(aHandle, created),
                ["name"] = created.Name ?? name,
                ["is_active"] = isActive,
                ["ignoring_constraints"] = created.IgnoringConstraints,
                ["native_tag"] = tag,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryArrangements(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var list = new List<object>();
            string activeName = string.Empty;
            if (part.ComponentAssembly != null)
            {
                var active = part.ComponentAssembly.ActiveArrangement;
                activeName = active != null ? (active.Name ?? string.Empty) : string.Empty;
                foreach (Arrangement a in part.ComponentAssembly.Arrangements.ToArray())
                {
                    var aHandle = Registry.Register(a, "Arrangement", ownerObjectId: partHandle.ObjectId);
                    uint tag = 0;
                    try { tag = (uint)a.Tag; } catch { }
                    list.Add(new Dictionary<string, object>
                    {
                        ["arrangement_ref"] = FormatHandle(aHandle, a),
                        ["name"] = a.Name ?? string.Empty,
                        ["is_active"] = (a == active),
                        ["ignoring_constraints"] = a.IgnoringConstraints,
                        ["native_tag"] = tag,
                    });
                }
            }
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["arrangements"] = list,
                ["active_arrangement_name"] = activeName,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblySetActiveArrangement(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var arrHandle = RequireHandle(payload, "arrangement_ref", "Arrangement");
        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var arrangement = (Arrangement)Registry.Resolve(arrHandle, "Arrangement");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");
            part.ComponentAssembly.ActiveArrangement = arrangement;
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["active_arrangement_name"] = arrangement.Name ?? string.Empty,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyDeleteArrangement(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var arrHandle = RequireHandle(payload, "arrangement_ref", "Arrangement");
        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var arrangement = (Arrangement)Registry.Resolve(arrHandle, "Arrangement");
            Journal.MarkStarted(requestId);
            arrangement.Delete(true);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["deleted"] = true,
            });
        }, token));
    }

    private static Task<byte[]> StartPartCreateReferenceSet(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var name = GetString(payload, "name", string.Empty);
        if (string.IsNullOrWhiteSpace(name)) throw new ArgumentException("name is required for part.create_reference_set");
        var memberHandles = ExtractHandleList(payload, "member_refs", string.Empty);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            var refSet = part.CreateReferenceSet();
            refSet.SetName(name);

            if (memberHandles.Count > 0)
            {
                var objs = new List<NXObject>();
                foreach (var h in memberHandles)
                {
                    var resolved = Registry.Resolve(h);
                    if (resolved is NXObject nxObj) objs.Add(nxObj);
                }
                if (objs.Count > 0)
                {
                    refSet.AddObjectsToReferenceSet(objs.ToArray());
                }
            }

            var rsHandle = Registry.Register(refSet, "ReferenceSet", ownerObjectId: partHandle.ObjectId);
            uint tag = 0;
            try { tag = (uint)refSet.Tag; } catch { }

            int memberCount = 0;
            try { memberCount = refSet.AskMembersInReferenceSet()?.Length ?? 0; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["reference_set_ref"] = FormatHandle(rsHandle, refSet),
                ["name"] = refSet.Name ?? name,
                ["member_count"] = memberCount,
                ["native_tag"] = tag,
            });
        }, token));
    }

    private static Task<byte[]> StartPartQueryReferenceSets(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var list = new List<object>();
            var sets = part.GetAllReferenceSets() ?? new ReferenceSet[0];
            foreach (var rs in sets)
            {
                var rsHandle = Registry.Register(rs, "ReferenceSet", ownerObjectId: partHandle.ObjectId);
                uint tag = 0;
                try { tag = (uint)rs.Tag; } catch { }
                int memberCount = 0;
                try { memberCount = rs.AskMembersInReferenceSet()?.Length ?? 0; } catch { }
                list.Add(new Dictionary<string, object>
                {
                    ["reference_set_ref"] = FormatHandle(rsHandle, rs),
                    ["name"] = rs.Name ?? string.Empty,
                    ["member_count"] = memberCount,
                    ["native_tag"] = tag,
                });
            }
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["reference_sets"] = list,
            });
        }, token));
    }

    private static Task<byte[]> StartPartModifyReferenceSetMembers(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var rsHandle = RequireHandle(payload, "reference_set_ref", "ReferenceSet");
        var addHandles = ExtractHandleList(payload, "add_member_refs", string.Empty);
        var removeHandles = ExtractHandleList(payload, "remove_member_refs", string.Empty);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var refSet = (ReferenceSet)Registry.Resolve(rsHandle, "ReferenceSet");
            Journal.MarkStarted(requestId);

            if (addHandles.Count > 0)
            {
                var toAdd = new List<NXObject>();
                foreach (var h in addHandles)
                {
                    var res = Registry.Resolve(h);
                    if (res is NXObject obj) toAdd.Add(obj);
                }
                if (toAdd.Count > 0) refSet.AddObjectsToReferenceSet(toAdd.ToArray());
            }

            if (removeHandles.Count > 0)
            {
                var toRemove = new List<NXObject>();
                foreach (var h in removeHandles)
                {
                    var res = Registry.Resolve(h);
                    if (res is NXObject obj) toRemove.Add(obj);
                }
                if (toRemove.Count > 0) refSet.RemoveObjectsFromReferenceSet(toRemove.ToArray());
            }

            int memberCount = 0;
            try { memberCount = refSet.AskMembersInReferenceSet()?.Length ?? 0; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["member_count"] = memberCount,
            });
        }, token));
    }

    private static Task<byte[]> StartPartDeleteReferenceSet(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var rsHandle = RequireHandle(payload, "reference_set_ref", "ReferenceSet");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var refSet = (ReferenceSet)Registry.Resolve(rsHandle, "ReferenceSet");
            Journal.MarkStarted(requestId);
            part.DeleteReferenceSet(refSet);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["deleted"] = true,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblySetComponentReferenceSet(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var compHandle = RequireHandle(payload, "component_ref", "Component");
        var refSetName = GetString(payload, "reference_set_name", string.Empty);
        if (string.IsNullOrWhiteSpace(refSetName)) throw new ArgumentException("reference_set_name is required");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var comp = (Component)Registry.Resolve(compHandle, "Component");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");
            part.ComponentAssembly.ReplaceReferenceSet(comp, refSetName);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["reference_set_name"] = comp.ReferenceSet ?? refSetName,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblySuppressComponents(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var compHandles = ExtractHandleList(payload, "component_refs", "Component");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");

            var comps = new List<Component>();
            foreach (var h in compHandles)
            {
                comps.Add((Component)Registry.Resolve(h, "Component"));
            }

            if (comps.Count > 0)
            {
                part.ComponentAssembly.SuppressComponents(comps.ToArray());
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["suppressed_count"] = comps.Count,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyUnsuppressComponents(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var compHandles = ExtractHandleList(payload, "component_refs", "Component");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");

            var comps = new List<Component>();
            foreach (var h in compHandles)
            {
                comps.Add((Component)Registry.Resolve(h, "Component"));
            }

            if (comps.Count > 0)
            {
                part.ComponentAssembly.UnsuppressComponents(comps.ToArray());
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["unsuppressed_count"] = comps.Count,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryComponentState(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var compHandle = RequireHandle(payload, "component_ref", "Component");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var comp = (Component)Registry.Resolve(compHandle, "Component");

            bool isSuppressed = false;
            try { isSuppressed = comp.IsSuppressed; } catch { }

            bool isLoaded = false;
            try { isLoaded = comp.Prototype != null; } catch { }

            string name = comp.DisplayName ?? comp.Name ?? string.Empty;
            string refSet = comp.ReferenceSet ?? string.Empty;
            uint tag = 0;
            try { tag = (uint)comp.Tag; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["component_ref"] = FormatHandle(compHandle, comp),
                ["name"] = name,
                ["is_suppressed"] = isSuppressed,
                ["is_loaded"] = isLoaded,
                ["reference_set"] = refSet,
                ["native_tag"] = tag,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyOpenComponents(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var compHandles = ExtractHandleList(payload, "component_refs", "Component");
        var optionStr = GetString(payload, "option", "whole_assembly");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");

            var comps = new List<Component>();
            foreach (var h in compHandles)
            {
                comps.Add((Component)Registry.Resolve(h, "Component"));
            }

            if (comps.Count > 0)
            {
                ComponentAssembly.OpenOption openOpt = ComponentAssembly.OpenOption.WholeAssembly;
                if (string.Equals(optionStr, "component_only", StringComparison.OrdinalIgnoreCase))
                {
                    openOpt = ComponentAssembly.OpenOption.ComponentOnly;
                }
                else if (string.Equals(optionStr, "immediate_children", StringComparison.OrdinalIgnoreCase))
                {
                    openOpt = ComponentAssembly.OpenOption.ImmediateChildren;
                }

                part.ComponentAssembly.OpenComponents(openOpt, comps.ToArray(), out ComponentAssembly.OpenComponentStatus[] _);
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["opened_count"] = comps.Count,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyCloseComponents(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var compHandles = ExtractHandleList(payload, "component_refs", "Component");
        var wholeTree = GetBool(payload, "whole_tree", false);
        var closeModified = GetBool(payload, "close_modified", false);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);
            if (part.ComponentAssembly == null) throw new InvalidOperationException("part has no component assembly");

            var comps = new List<Component>();
            foreach (var h in compHandles)
            {
                comps.Add((Component)Registry.Resolve(h, "Component"));
            }

            if (comps.Count > 0)
            {
                var wt = wholeTree ? BasePart.CloseWholeTree.True : BasePart.CloseWholeTree.False;
                var cm = closeModified ? ComponentAssembly.CloseModified.True : ComponentAssembly.CloseModified.False;
                part.ComponentAssembly.CloseComponents(comps.ToArray(), wt, cm);
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["closed_count"] = comps.Count,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryInterpartReferences(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var parents = part.GetInterpartParents() ?? new Part[0];
            var children = part.GetInterpartChildren() ?? new Part[0];

            var parentList = new List<object>();
            foreach (var p in parents)
            {
                var h = Registry.Register(p, "Part", ownerObjectId: partHandle.ObjectId);
                parentList.Add(new Dictionary<string, object>
                {
                    ["part_ref"] = FormatHandle(h, p),
                    ["part_path"] = (p.FullPath ?? string.Empty).Replace('\\', '/'),
                    ["part_name"] = p.Leaf ?? p.Name ?? string.Empty,
                    ["native_tag"] = (uint)p.Tag,
                    ["direction"] = "parent",
                });
            }

            var childList = new List<object>();
            foreach (var c in children)
            {
                var h = Registry.Register(c, "Part", ownerObjectId: partHandle.ObjectId);
                childList.Add(new Dictionary<string, object>
                {
                    ["part_ref"] = FormatHandle(h, c),
                    ["part_path"] = (c.FullPath ?? string.Empty).Replace('\\', '/'),
                    ["part_name"] = c.Leaf ?? c.Name ?? string.Empty,
                    ["native_tag"] = (uint)c.Tag,
                    ["direction"] = "child",
                });
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["part_ref"] = FormatHandle(partHandle, part),
                ["parents"] = parentList,
                ["children"] = childList,
                ["total_count"] = parentList.Count + childList.Count,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyGetInterpartPolicy(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            bool delay = false;
            try { delay = session.UpdateManager?.InterpartDelay ?? false; } catch { }

            string dataOptStr = "detect_out_of_date_only";
            string parentOptStr = "all";
            try
            {
                session.Parts.LoadOptions.GetInterpartDataOption(out var dataOpt, out var parentOpt);
                switch (dataOpt)
                {
                    case LoadOptions.InterpartDataOption.DetectOutOfDateAndLoad:
                        dataOptStr = "detect_out_of_date_and_load";
                        break;
                    case LoadOptions.InterpartDataOption.NoDetectAndNoLoad:
                        dataOptStr = "no_detect_and_no_load";
                        break;
                    default:
                        dataOptStr = "detect_out_of_date_only";
                        break;
                }
                switch (parentOpt)
                {
                    case LoadOptions.Parent.Immediate:
                        parentOptStr = "immediate";
                        break;
                    case LoadOptions.Parent.Partial:
                        parentOptStr = "partial";
                        break;
                    default:
                        parentOptStr = "all";
                        break;
                }
            }
            catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["interpart_delay"] = delay,
                ["interpart_data_option"] = dataOptStr,
                ["parent_load_option"] = parentOptStr,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblySetInterpartPolicy(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId);

            if (payload.TryGetValue("interpart_delay", out var dRaw) && dRaw is bool d)
            {
                try { if (session.UpdateManager != null) session.UpdateManager.InterpartDelay = d; } catch { }
            }

            session.Parts.LoadOptions.GetInterpartDataOption(out var curDataOpt, out var curParentOpt);

            if (payload.TryGetValue("interpart_data_option", out var optRaw) && optRaw is string optStr)
            {
                if (string.Equals(optStr, "detect_out_of_date_and_load", StringComparison.OrdinalIgnoreCase))
                    curDataOpt = LoadOptions.InterpartDataOption.DetectOutOfDateAndLoad;
                else if (string.Equals(optStr, "no_detect_and_no_load", StringComparison.OrdinalIgnoreCase))
                    curDataOpt = LoadOptions.InterpartDataOption.NoDetectAndNoLoad;
                else if (string.Equals(optStr, "detect_out_of_date_only", StringComparison.OrdinalIgnoreCase))
                    curDataOpt = LoadOptions.InterpartDataOption.DetectOutOfDateOnly;
            }

            if (payload.TryGetValue("parent_load_option", out var pRaw) && pRaw is string pStr)
            {
                if (string.Equals(pStr, "immediate", StringComparison.OrdinalIgnoreCase))
                    curParentOpt = LoadOptions.Parent.Immediate;
                else if (string.Equals(pStr, "partial", StringComparison.OrdinalIgnoreCase))
                    curParentOpt = LoadOptions.Parent.Partial;
                else if (string.Equals(pStr, "all", StringComparison.OrdinalIgnoreCase))
                    curParentOpt = LoadOptions.Parent.All;
            }

            try { session.Parts.LoadOptions.SetInterpartDataOption(curDataOpt, curParentOpt); } catch { }

            bool delay = false;
            try { delay = session.UpdateManager?.InterpartDelay ?? false; } catch { }

            string outDataOpt = curDataOpt == LoadOptions.InterpartDataOption.DetectOutOfDateAndLoad ? "detect_out_of_date_and_load" :
                (curDataOpt == LoadOptions.InterpartDataOption.NoDetectAndNoLoad ? "no_detect_and_no_load" : "detect_out_of_date_only");
            string outParentOpt = curParentOpt == LoadOptions.Parent.Immediate ? "immediate" :
                (curParentOpt == LoadOptions.Parent.Partial ? "partial" : "all");

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["interpart_delay"] = delay,
                ["interpart_data_option"] = outDataOpt,
                ["parent_load_option"] = outParentOpt,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyUpdateInterpartReferences(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var scope = GetString(payload, "scope", "session");
        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId);
            var mark = session.SetUndoMark(Session.MarkVisibility.Invisible, "NXGO_InterpartUpdate");
            try
            {
                if (string.Equals(scope, "assembly", StringComparison.OrdinalIgnoreCase))
                {
                    session.UpdateManager?.DoAssemblyInterpartUpdate(mark);
                }
                else
                {
                    session.UpdateManager?.DoInterpartUpdate(mark);
                }
            }
            finally
            {
                session.DeleteUndoMark(mark, "NXGO_InterpartUpdate");
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["updated"] = true,
            });
        }, token));
    }

    private static Task<byte[]> StartAssemblyQueryBulk(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "assembly_part_ref", "Part");
        var maxDepth = GetInt(payload, "max_depth", -1);
        var includeSuppressed = GetBool(payload, "include_suppressed", true);
        var includeTransforms = GetBool(payload, "include_transforms", true);
        var includeBoundingBox = GetBool(payload, "include_bounding_box", false);
        var nameFilter = GetString(payload, "name_filter", string.Empty);
        var offset = GetInt(payload, "offset", 0);
        var limit = GetInt(payload, "limit", 5000);
        if (limit <= 0 || limit > 10000) limit = 5000;

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var root = part.ComponentAssembly != null ? part.ComponentAssembly.RootComponent : null;
            if (root == null) throw new InvalidOperationException("assembly root component is unavailable");

            var uniquePartPaths = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            var resultItems = new List<object>();
            int totalComponents = 0;
            int totalLoaded = 0;
            int totalSuppressed = 0;
            bool hasMore = false;
            int matchedIndex = 0;

            UFSession? uf = null;
            if (includeBoundingBox)
            {
                try { uf = UFSession.GetUFSession(); } catch { }
            }

            var queue = new Queue<(Component comp, int depth)>();
            queue.Enqueue((root, 0));
            int nodesVisited = 0;

            while (queue.Count > 0)
            {
                var (current, depth) = queue.Dequeue();
                nodesVisited++;
                if (nodesVisited > MaxAssemblySnapshotNodes) break;

                Component[] children;
                try { children = current.GetChildren() ?? new Component[0]; }
                catch { children = new Component[0]; }

                if (maxDepth < 0 || depth < maxDepth)
                {
                    foreach (var child in children)
                    {
                        queue.Enqueue((child, depth + 1));
                    }
                }

                if (depth == 0) continue;

                totalComponents++;

                bool isSuppressed = false;
                try { isSuppressed = current.IsSuppressed; } catch { }
                if (isSuppressed) totalSuppressed++;

                bool isLoaded = false;
                try { isLoaded = current.Prototype != null; } catch { }
                if (isLoaded) totalLoaded++;

                var path = ComponentPrototypePath(current);
                if (!string.IsNullOrWhiteSpace(path))
                {
                    uniquePartPaths.Add(path);
                }

                if (!includeSuppressed && isSuppressed)
                {
                    continue;
                }

                var compName = current.Name ?? string.Empty;
                var displayName = current.DisplayName ?? compName;

                if (!string.IsNullOrEmpty(nameFilter))
                {
                    if (!compName.Contains(nameFilter, StringComparison.OrdinalIgnoreCase) &&
                        !displayName.Contains(nameFilter, StringComparison.OrdinalIgnoreCase))
                    {
                        continue;
                    }
                }

                if (matchedIndex >= offset && resultItems.Count < limit)
                {
                    var item = new Dictionary<string, object>
                    {
                        ["name"] = compName,
                        ["display_name"] = displayName,
                        ["part_path"] = path.Replace('\\', '/'),
                        ["part_name"] = Path.GetFileName(path),
                        ["depth"] = depth,
                        ["is_suppressed"] = isSuppressed,
                        ["is_loaded"] = isLoaded,
                        ["reference_set"] = current.ReferenceSet ?? string.Empty,
                        ["native_tag"] = (uint)current.Tag,
                        ["children_count"] = children.Length,
                    };

                    if (includeTransforms)
                    {
                        Point3d pos;
                        Matrix3x3 ori;
                        try
                        {
                            current.GetPosition(out pos, out ori);
                            item["position"] = new[] { pos.X, pos.Y, pos.Z };
                            item["orientation"] = new[]
                            {
                                ori.Xx, ori.Xy, ori.Xz,
                                ori.Yx, ori.Yy, ori.Yz,
                                ori.Zx, ori.Zy, ori.Zz
                            };
                        }
                        catch
                        {
                            item["position"] = new[] { 0.0, 0.0, 0.0 };
                            item["orientation"] = new[] { 1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0 };
                        }
                    }

                    if (includeBoundingBox && uf != null)
                    {
                        try
                        {
                            double[] minMax = new double[6];
                            uf.Modl.AskBoundingBox(current.Tag, minMax);
                            item["box_min"] = new[] { minMax[0], minMax[1], minMax[2] };
                            item["box_max"] = new[] { minMax[3], minMax[4], minMax[5] };
                        }
                        catch { }
                    }

                    resultItems.Add(item);
                }
                else if (matchedIndex >= offset + limit)
                {
                    hasMore = true;
                }

                matchedIndex++;
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["assembly_part_ref"] = FormatHandle(partHandle, part),
                ["total_components"] = totalComponents,
                ["total_loaded"] = totalLoaded,
                ["total_suppressed"] = totalSuppressed,
                ["unique_parts_count"] = uniquePartPaths.Count,
                ["components"] = resultItems,
                ["has_more"] = hasMore,
            });
        }, token));
    }
}
