using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;
using NXOpen.Assemblies;

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
            List<string> names;
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
        object value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        return Convert.ToInt32(value, CultureInfo.InvariantCulture);
    }
}
