using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;

namespace NXGO.Agent.NXHost;

public static partial class EntryPoint
{
    private const int MaxProducedHandlesPerRequest = 256;

    private static Task<byte[]> StartCreateBlock(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        RequireCreateOnlyFeatureOptions(payload);
        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var length = GetDouble(payload, "length", 100.0);
        var width = GetDouble(payload, "width", 100.0);
        var height = GetDouble(payload, "height", 100.0);
        if (length <= 0 || width <= 0 || height <= 0) throw new ArgumentException("block dimensions must be positive");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            RequireMatchingPartUnits(payload, part);
            Journal.MarkStarted(requestId);
            using (var scope = new BuilderScope<NXOpen.Features.BlockFeatureBuilder>(
                part.Features.CreateBlockFeatureBuilder(null),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Type = NXOpen.Features.BlockFeatureBuilder.Types.OriginAndEdgeLengths;
                builder.SetOriginAndLengths(
                    new Point3d(origin[0], origin[1], origin[2]),
                    length.ToString("G", CultureInfo.InvariantCulture),
                    width.ToString("G", CultureInfo.InvariantCulture),
                    height.ToString("G", CultureInfo.InvariantCulture));
                builder.BooleanOption.Type = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Create;

                var feature = scope.CommitOnce(b => (NXOpen.Features.BodyFeature)b.CommitFeature());
                var bodies = feature.GetBodies();
                if (bodies == null || bodies.Length == 0) throw new InvalidOperationException("block feature commit produced no body");
                var body = bodies != null && bodies.Length > 0 ? bodies[0] : null;
                var featureHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);
                object bodyWire = new Dictionary<string, object>();
                if (body != null)
                {
                    var bodyHandle = Registry.Register(body, "Body", ownerObjectId: partHandle.ObjectId);
                    bodyWire = FormatHandle(bodyHandle, body);
                }

                return FormatResponse(requestId, new Dictionary<string, object>
                {
                    ["feature_ref"] = FormatHandle(featureHandle, feature),
                    ["body_ref"] = bodyWire,
                    ["feature_name"] = feature.GetFeatureName(),
                    ["feature_type"] = feature.FeatureType,
                });
            }
        }, token));
    }

    private static Task<byte[]> StartCreateCylinder(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        RequireCreateOnlyFeatureOptions(payload);
        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var direction = GetDoubleArray(payload, "direction", 3, new[] { 0.0, 0.0, 1.0 });
        if (direction[0] == 0 && direction[1] == 0 && direction[2] == 0) direction[2] = 1.0;
        var diameter = GetDouble(payload, "diameter", 50.0);
        var height = GetDouble(payload, "height", 100.0);
        if (diameter <= 0 || height <= 0) throw new ArgumentException("cylinder diameter and height must be positive");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            RequireMatchingPartUnits(payload, part);
            Journal.MarkStarted(requestId);
            using (var scope = new BuilderScope<NXOpen.Features.CylinderBuilder>(
                part.Features.CreateCylinderBuilder(null),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Type = NXOpen.Features.CylinderBuilder.Types.AxisDiameterAndHeight;
                builder.Diameter.RightHandSide = diameter.ToString("G", CultureInfo.InvariantCulture);
                builder.Height.RightHandSide = height.ToString("G", CultureInfo.InvariantCulture);
                builder.Origin = new Point3d(origin[0], origin[1], origin[2]);
                builder.Direction = new Vector3d(direction[0], direction[1], direction[2]);
                builder.BooleanOption.Type = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Create;

                var feature = scope.CommitOnce(b => (NXOpen.Features.BodyFeature)b.CommitFeature());
                var bodies = feature.GetBodies();
                if (bodies == null || bodies.Length == 0) throw new InvalidOperationException("cylinder feature commit produced no body");
                var body = bodies != null && bodies.Length > 0 ? bodies[0] : null;
                var featureHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);
                object bodyWire = new Dictionary<string, object>();
                if (body != null)
                {
                    var bodyHandle = Registry.Register(body, "Body", ownerObjectId: partHandle.ObjectId);
                    bodyWire = FormatHandle(bodyHandle, body);
                }

                return FormatResponse(requestId, new Dictionary<string, object>
                {
                    ["feature_ref"] = FormatHandle(featureHandle, feature),
                    ["body_ref"] = bodyWire,
                    ["feature_name"] = feature.GetFeatureName(),
                    ["feature_type"] = feature.FeatureType,
                });
            }
        }, token));
    }

    private static void RequireMatchingPartUnits(Dictionary<string, object> payload, Part part)
    {
        var requested = GetString(payload, "units", "").Trim().ToLowerInvariant();
        var actual = part.PartUnits == (BasePart.Units)Part.Units.Inches ? "inch" : "mm";
        if (requested == "millimeters") requested = "mm";
        if (requested == "inches") requested = "inch";
        if (requested != actual)
            throw new ArgumentException("feature dimensions units must match owning part units");
    }

    private static Task<byte[]> StartQueryBodies(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var bodies = new List<Body>();
            foreach (Body body in part.Bodies) bodies.Add(body);
            if (bodies.Count > MaxProducedHandlesPerRequest)
            {
                throw new HandleScopeCapacityException(requestId, MaxProducedHandlesPerRequest);
            }
            if (Registry.Count + bodies.Count > Registry.Capacity)
            {
                throw new HandleRegistryCapacityException(Registry.Capacity);
            }

            var result = new List<object>();
            foreach (var body in bodies)
            {
                var handle = Registry.Register(
                    body,
                    "Body",
                    leaseScopeId: requestId,
                    ownerObjectId: partHandle.ObjectId,
                    leaseScopeLimit: MaxProducedHandlesPerRequest);
                var faceCount = 0;
                foreach (Face _ in body.GetFaces()) faceCount++;
                var edgeCount = 0;
                foreach (Edge _ in body.GetEdges()) edgeCount++;
                uint nativeTag = 0;
                try { nativeTag = (uint)body.Tag; } catch { }

                result.Add(new Dictionary<string, object>
                {
                    ["body_ref"] = FormatHandle(handle, body),
                    ["name"] = body.Name,
                    ["solid_type"] = body.IsSolidBody ? "solid" : "sheet",
                    ["face_count"] = faceCount,
                    ["edge_count"] = edgeCount,
                    ["native_tag"] = nativeTag,
                });
            }
            return FormatResponse(requestId, new Dictionary<string, object> { ["bodies"] = result });
        }, token));
    }

    private static Task<byte[]> StartMassProperties(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var bodyHandle = RequireHandle(payload, "body_ref", "Body");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var body = (Body)Registry.Resolve(bodyHandle, "Body");
            var contract = ContractFor(body);
            var uf = NXOpen.UF.UFSession.GetUFSession();
            var bodyTags = new NXOpen.Tag[] { body.Tag };
            var accuracyValues = new double[11];
            var massProps = new double[47];
            var statistics = new double[13];
            uf.Modl.AskMassProps3d(
                bodyTags,
                1,
                1,
                (int)contract.UfMassUnits,
                1.0,
                1,
                accuracyValues,
                massProps,
                statistics);

            var normalized = contract.NormalizeMassProperties(massProps);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
	            ["units"] = contract.PartLengthUnit == NxgoLengthUnit.Inch ? "inch" : "mm",
                ["volume"] = normalized.Volume,
                ["area"] = normalized.Area,
                ["mass"] = normalized.Mass,
                ["centroid"] = normalized.Centroid,
                ["solid_type"] = body.IsSolidBody ? "solid" : "sheet",
            });
        }, token));
    }

    private static Task<byte[]> StartBoundingBox(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var bodyHandle = RequireHandle(payload, "body_ref", "Body");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var body = (Body)Registry.Resolve(bodyHandle, "Body");
            var uf = NXOpen.UF.UFSession.GetUFSession();
            var minMax = new double[6];
            uf.Modl.AskBoundingBox(body.Tag, minMax);
            var normalized = ContractFor(body).NormalizeBoundingBox(minMax);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
	            ["units"] = ContractFor(body).PartLengthUnit == NxgoLengthUnit.Inch ? "inch" : "mm",
                ["min_corner"] = normalized.MinCorner,
                ["max_corner"] = normalized.MaxCorner,
                ["dimensions"] = normalized.Dimensions,
            });
        }, token));
    }

    private static Task<byte[]> StartObjectRelease(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var rawHandles = GetArray(payload, "handles");
        var handles = new List<ObjectHandleToken>();
        var identities = new HashSet<string>(StringComparer.Ordinal);
        foreach (var raw in rawHandles)
        {
            var dict = raw as Dictionary<string, object>;
            if (dict == null) throw new ArgumentException("each handles item must be an object");
            var handle = ParseHandle(dict);
            var identity = handle.ObjectId + "/" + handle.Generation.ToString(CultureInfo.InvariantCulture);
            if (!identities.Add(identity)) throw new ArgumentException("duplicate handle in object.release: " + identity);
            handles.Add(handle);
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            foreach (var handle in handles)
            {
                Registry.Resolve(handle);
            }
            Journal.MarkStarted(requestId);
            var released = 0;
            foreach (var handle in handles)
            {
                if (Registry.Release(handle)) released++;
            }
            return FormatResponse(requestId, new Dictionary<string, object> { ["released_count"] = released });
        }, token));
    }

    private static GeometryUnitContract ContractFor(Body body)
    {
        var imperial = body.OwningPart != null && body.OwningPart.PartUnits == Part.Units.Inches;
        return imperial ? GeometryUnitContract.InchPound : GeometryUnitContract.MillimeterKilogram;
    }

    private static void RequireCreateOnlyFeatureOptions(Dictionary<string, object> payload)
    {
        var booleanOp = GetString(payload, "boolean_op", "create").Trim().ToLowerInvariant();
        if (booleanOp.Length > 0 && booleanOp != "create")
        {
            throw new ArgumentException("boolean operation is not implemented by the canonical backend; only create is accepted");
        }
        object target;
        if (payload.TryGetValue("target_body_ref", out target) && target != null)
        {
            throw new ArgumentException("target_body_ref is not implemented by the canonical backend");
        }
    }

    private static double GetDouble(Dictionary<string, object> source, string key, double defaultValue)
    {
        object value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        return Convert.ToDouble(value, CultureInfo.InvariantCulture);
    }

    private static double[] GetDoubleArray(Dictionary<string, object> source, string key, int requiredLength, double[] defaultValue)
    {
        object raw;
        if (!source.TryGetValue(key, out raw) || raw == null) return (double[])defaultValue.Clone();
        var items = ToObjectArray(raw);
        if (items.Length != requiredLength) throw new ArgumentException(key + " must contain exactly " + requiredLength + " numbers");
        var result = new double[requiredLength];
        for (var i = 0; i < requiredLength; i++) result[i] = Convert.ToDouble(items[i], CultureInfo.InvariantCulture);
        return result;
    }

    private static object[] GetArray(Dictionary<string, object> source, string key)
    {
        object raw;
        if (!source.TryGetValue(key, out raw) || raw == null) return new object[0];
        return ToObjectArray(raw);
    }

    private static object[] ToObjectArray(object raw)
    {
        var direct = raw as object[];
        if (direct != null) return direct;
        var list = raw as ArrayList;
        if (list != null) return list.Cast<object>().ToArray();
        var enumerable = raw as IEnumerable<object>;
        if (enumerable != null) return enumerable.ToArray();
        throw new ArgumentException("JSON field must be an array");
    }

    private static ObjectHandleToken ParseHandle(Dictionary<string, object> raw)
    {
        return new ObjectHandleToken
        {
            SessionId = GetString(raw, "session_id", string.Empty),
            Epoch = GetUInt64(raw, "epoch"),
            ObjectId = GetString(raw, "object_id", string.Empty),
            Generation = GetUInt32(raw, "generation"),
            Kind = GetString(raw, "kind", string.Empty),
            LeaseScopeId = GetString(raw, "lease_scope_id", string.Empty),
        };
    }
}
