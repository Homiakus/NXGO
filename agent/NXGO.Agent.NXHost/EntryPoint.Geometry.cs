using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;
using NXOpen.UF;

namespace NXGO.Agent.NXHost;

public static partial class EntryPoint
{
    private const int MaxProducedHandlesPerRequest = 256;

    private static Task<byte[]> StartCreateBlock(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        ValidateBooleanFeatureOptions(payload);
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
                part.Features.CreateBlockFeatureBuilder(null!),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Type = NXOpen.Features.BlockFeatureBuilder.Types.OriginAndEdgeLengths;
                builder.SetOriginAndLengths(
                    new Point3d(origin[0], origin[1], origin[2]),
                    length.ToString("G", CultureInfo.InvariantCulture),
                    width.ToString("G", CultureInfo.InvariantCulture),
                    height.ToString("G", CultureInfo.InvariantCulture));
                ApplyBooleanOption(part, builder.BooleanOption, payload);

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
        ValidateBooleanFeatureOptions(payload);
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
                part.Features.CreateCylinderBuilder(null!),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Type = NXOpen.Features.CylinderBuilder.Types.AxisDiameterAndHeight;
                builder.Diameter.RightHandSide = diameter.ToString("G", CultureInfo.InvariantCulture);
                builder.Height.RightHandSide = height.ToString("G", CultureInfo.InvariantCulture);
                builder.Origin = new Point3d(origin[0], origin[1], origin[2]);
                builder.Direction = new Vector3d(direction[0], direction[1], direction[2]);
                ApplyBooleanOption(part, builder.BooleanOption, payload);

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
        var leaseScopeId = GetString(payload, "lease_scope_id", "").Trim();
        var rawHandles = GetArray(payload, "handles");
        if (leaseScopeId.Length > 0 && rawHandles.Length > 0)
            throw new ArgumentException("object.release accepts lease_scope_id or handles, not both");
        if (leaseScopeId.Length == 0 && rawHandles.Length == 0)
            throw new ArgumentException("object.release requires lease_scope_id or handles");
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
            if (leaseScopeId.Length > 0)
            {
                Journal.MarkStarted(requestId); PersistJournalOrThrow();
                var releasedByScope = Registry.ReleaseScope(leaseScopeId);
                return FormatResponse(requestId, new Dictionary<string, object> { ["released_count"] = releasedByScope });
            }
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
        var imperial = body.OwningPart != null && body.OwningPart.PartUnits == (BasePart.Units)Part.Units.Inches;
        return imperial ? GeometryUnitContract.InchPound : GeometryUnitContract.MillimeterKilogram;
    }

    private static void ValidateBooleanFeatureOptions(Dictionary<string, object> payload)
    {
        var booleanOp = GetString(payload, "boolean_op", "create").Trim().ToLowerInvariant();
        if (string.IsNullOrEmpty(booleanOp) || booleanOp == "create")
        {
            if (payload.TryGetValue("target_body_ref", out var target) && target != null)
            {
                throw new ArgumentException("target_body_ref cannot be specified with boolean create");
            }
            return;
        }

        switch (booleanOp)
        {
            case "unite":
            case "subtract":
            case "intersect":
                if (!payload.ContainsKey("target_body_ref") || payload["target_body_ref"] == null)
                {
                    throw new ArgumentException("target_body_ref is required for boolean operation " + booleanOp);
                }
                RequireHandle(payload, "target_body_ref", "Body");
                break;
            default:
                throw new ArgumentException("unsupported boolean operation: " + booleanOp + "; supported: create, unite, subtract, intersect");
        }
    }

    private static void ApplyBooleanOption(Part part, NXOpen.GeometricUtilities.BooleanOperation booleanOption, Dictionary<string, object> payload)
    {
        var booleanOp = GetString(payload, "boolean_op", "create").Trim().ToLowerInvariant();
        if (string.IsNullOrEmpty(booleanOp) || booleanOp == "create")
        {
            booleanOption.Type = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Create;
            return;
        }

        NXOpen.GeometricUtilities.BooleanOperation.BooleanType boolType;
        switch (booleanOp)
        {
            case "unite":
                boolType = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Unite;
                break;
            case "subtract":
                boolType = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Subtract;
                break;
            case "intersect":
                boolType = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Intersect;
                break;
            default:
                throw new ArgumentException("unsupported boolean operation: " + booleanOp);
        }

        var targetHandle = RequireHandle(payload, "target_body_ref", "Body");
        var targetBody = (Body)Registry.Resolve(targetHandle, "Body");
        booleanOption.SetBooleanOperationAndBody(boolType, targetBody);
    }

    private static Task<byte[]> StartBooleanOperation(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var targetHandle = RequireHandle(payload, "target_body_ref", "Body");
        var toolHandles = ExtractHandleList(payload, "tool_body_refs", "Body");
        if (toolHandles.Count == 0) throw new ArgumentException("at least one tool body is required for boolean operation");
        var opStr = GetString(payload, "op", "unite").Trim().ToLowerInvariant();

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var targetBody = (Body)Registry.Resolve(targetHandle, "Body");
            var toolBodies = toolHandles.Select(h => (Body)Registry.Resolve(h, "Body")).ToArray();

            Journal.MarkStarted(requestId);
            NXOpen.Features.BooleanFeature[] boolFeatures;
            bool nonAssoc = false;
            bool unparam = false;

            switch (opStr)
            {
                case "unite":
                    boolFeatures = part.Features.CreateUniteFeature(targetBody, false, toolBodies, false, false, out nonAssoc, out unparam);
                    break;
                case "subtract":
                    boolFeatures = part.Features.CreateSubtractFeature(targetBody, false, toolBodies, false, false, out nonAssoc, out unparam);
                    break;
                default:
                    throw new ArgumentException("standalone boolean operation currently supports 'unite' and 'subtract': " + opStr);
            }

            if (boolFeatures == null || boolFeatures.Length == 0)
            {
                throw new InvalidOperationException("boolean operation produced no feature");
            }

            var feature = boolFeatures[0];
            var featureHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);
            var bodies = feature.GetBodies();
            var body = bodies != null && bodies.Length > 0 ? bodies[0] : targetBody;
            var bodyHandle = Registry.Register(body, "Body", ownerObjectId: partHandle.ObjectId);

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["feature_ref"] = FormatHandle(featureHandle, feature),
                ["body_ref"] = FormatHandle(bodyHandle, body),
                ["feature_name"] = feature.GetFeatureName(),
                ["feature_type"] = feature.FeatureType,
            });
        }, token));
    }

    private static Task<byte[]> StartCreateHole(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var targetHandle = RequireHandle(payload, "target_body_ref", "Body");
        ObjectHandleToken? faceHandle = null;
        if (payload.TryGetValue("face_ref", out var rawFace) && rawFace is Dictionary<string, object> faceDict && faceDict.Count > 0)
        {
            faceHandle = ParseHandle(faceDict);
        }

        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var direction = GetDoubleArray(payload, "direction", 3, new[] { 0.0, 0.0, -1.0 });
        if (direction[0] == 0 && direction[1] == 0 && direction[2] == 0) direction[2] = -1.0;

        var diameter = GetDouble(payload, "diameter", 10.0);
        var depth = GetDouble(payload, "depth", 25.0);
        var tipAngle = GetDouble(payload, "tip_angle", 118.0);
        var throughBody = GetBool(payload, "through_body", false);
        var holeType = GetString(payload, "hole_type", "simple").Trim().ToLowerInvariant();

        var cbDia = GetDouble(payload, "counterbore_diameter", diameter * 1.5);
        var cbDepth = GetDouble(payload, "counterbore_depth", 5.0);
        var csDia = GetDouble(payload, "countersink_diameter", diameter * 1.5);
        var csAngle = GetDouble(payload, "countersink_angle", 90.0);

        if (diameter <= 0) throw new ArgumentException("hole diameter must be positive");
        if (!throughBody && depth <= 0) throw new ArgumentException("hole depth must be positive for blind hole");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            RequireMatchingPartUnits(payload, part);
            var targetBody = (Body)Registry.Resolve(targetHandle, "Body");
            Journal.MarkStarted(requestId);

            var uf = NXOpen.UF.UFSession.GetUFSession();
            Tag placementFace = Tag.Null;
            Tag throughFace = Tag.Null;

            if (faceHandle != null)
            {
                var faceObj = (Face)Registry.Resolve(faceHandle, "Face");
                placementFace = faceObj.Tag;
            }
            else
            {
                var faces = targetBody.GetFaces();
                if (faces != null && faces.Length > 0)
                {
                    try
                    {
                        uf.Modl.TraceARay(1, new[] { targetBody.Tag }, origin, direction, null!, 1, out int numHits, out var hitList);
                        if (numHits > 0 && hitList != null && hitList.Length > 0)
                        {
                            placementFace = hitList[0].hit_face;
                        }
                    }
                    catch
                    {
                        // Fallback if ray trace is unavailable in current mode
                    }

                    if (placementFace == Tag.Null)
                    {
                        placementFace = faces[0].Tag;
                    }
                }
            }

            if (placementFace == Tag.Null)
            {
                throw new InvalidOperationException("target body has no faces available for hole placement");
            }

            var diaStr = diameter.ToString("G", CultureInfo.InvariantCulture);
            var depthStr = (throughBody ? Math.Max(depth, 10000.0) : depth).ToString("G", CultureInfo.InvariantCulture);
            var tipAngleStr = tipAngle.ToString("G", CultureInfo.InvariantCulture);
            Tag featureTag;

            switch (holeType)
            {
                case "counterbore":
                    var cbDiaStr = cbDia.ToString("G", CultureInfo.InvariantCulture);
                    var cbDepthStr = cbDepth.ToString("G", CultureInfo.InvariantCulture);
                    uf.Modl.CreateCBoreHole(origin, direction, diaStr, depthStr, cbDiaStr, cbDepthStr, tipAngleStr, placementFace, throughFace, out featureTag);
                    break;
                case "countersink":
                    var csDiaStr = csDia.ToString("G", CultureInfo.InvariantCulture);
                    var csAngleStr = csAngle.ToString("G", CultureInfo.InvariantCulture);
                    uf.Modl.CreateCSunkHole(origin, direction, diaStr, depthStr, csDiaStr, csAngleStr, tipAngleStr, placementFace, throughFace, out featureTag);
                    break;
                case "simple":
                default:
                    uf.Modl.CreateSimpleHole(origin, direction, diaStr, depthStr, tipAngleStr, placementFace, throughFace, out featureTag);
                    break;
            }

            if (featureTag == Tag.Null)
            {
                throw new InvalidOperationException("NX UFModl hole creation failed to return a valid feature tag");
            }

            var feature = (NXOpen.Features.Feature)NXOpen.Utilities.NXObjectManager.Get(featureTag);
            var featureHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);
            var bodies = feature.GetBodies();
            var body = bodies != null && bodies.Length > 0 ? bodies[0] : targetBody;
            var bodyHandle = Registry.Register(body, "Body", ownerObjectId: partHandle.ObjectId);

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["feature_ref"] = FormatHandle(featureHandle, feature),
                ["body_ref"] = FormatHandle(bodyHandle, body),
                ["feature_name"] = feature.GetFeatureName(),
                ["feature_type"] = feature.FeatureType,
            });
        }, token));
    }

    private static Task<byte[]> StartDatumCreatePlane(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var direction = GetDoubleArray(payload, "direction", 3, new[] { 0.0, 0.0, 1.0 });
        if (direction[0] == 0 && direction[1] == 0 && direction[2] == 0) direction[2] = 1.0;

        var len = Math.Sqrt(direction[0] * direction[0] + direction[1] * direction[1] + direction[2] * direction[2]);
        var nz = direction[2] / len;
        var nx = direction[0] / len;
        var ny = direction[1] / len;

        double tx = Math.Abs(nx) < 0.9 && Math.Abs(ny) < 0.9 ? 0 : 1;
        double ty = Math.Abs(nx) < 0.9 && Math.Abs(ny) < 0.9 ? 0 : 0;
        double tz = Math.Abs(nx) < 0.9 && Math.Abs(ny) < 0.9 ? 1 : 0;

        double ux = ty * nz - tz * ny;
        double uy = tz * nx - tx * nz;
        double uz = tx * ny - ty * nx;
        var ulen = Math.Sqrt(ux * ux + uy * uy + uz * uz);
        ux /= ulen; uy /= ulen; uz /= ulen;

        double vx = ny * uz - nz * uy;
        double vy = nz * ux - nx * uz;
        double vz = nx * uy - ny * ux;

        var matrix = new Matrix3x3
        {
            Xx = ux, Xy = uy, Xz = uz,
            Yx = vx, Yy = vy, Yz = vz,
            Zx = nx, Zy = ny, Zz = nz
        };

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            var plane = part.Datums.CreateFixedDatumPlane(new Point3d(origin[0], origin[1], origin[2]), matrix);
            var planeHandle = Registry.Register(plane, "DatumPlane", ownerObjectId: partHandle.ObjectId);

            var feat = plane.Feature;
            var featHandle = feat != null ? Registry.Register(feat, "Feature", ownerObjectId: partHandle.ObjectId) : null;

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["plane_ref"] = FormatHandle(planeHandle, plane),
                ["feature_ref"] = featHandle != null ? FormatHandle(featHandle, feat!) : new Dictionary<string, object>(),
                ["name"] = feat != null ? feat.GetFeatureName() : (plane.Name ?? "DatumPlane"),
            });
        }, token));
    }

    private static Task<byte[]> StartDatumCreateAxis(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var direction = GetDoubleArray(payload, "direction", 3, new[] { 0.0, 0.0, 1.0 });
        if (direction[0] == 0 && direction[1] == 0 && direction[2] == 0) direction[2] = 1.0;

        var start = new Point3d(origin[0], origin[1], origin[2]);
        var end = new Point3d(origin[0] + direction[0] * 100.0, origin[1] + direction[1] * 100.0, origin[2] + direction[2] * 100.0);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            var axis = part.Datums.CreateFixedDatumAxis(start, end);
            var axisHandle = Registry.Register(axis, "DatumAxis", ownerObjectId: partHandle.ObjectId);

            var feat = axis.Feature;
            var featHandle = feat != null ? Registry.Register(feat, "Feature", ownerObjectId: partHandle.ObjectId) : null;

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["axis_ref"] = FormatHandle(axisHandle, axis),
                ["feature_ref"] = featHandle != null ? FormatHandle(featHandle, feat!) : new Dictionary<string, object>(),
                ["name"] = feat != null ? feat.GetFeatureName() : (axis.Name ?? "DatumAxis"),
            });
        }, token));
    }

    private static Task<byte[]> StartDatumCreateCsys(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var origin = GetDoubleArray(payload, "origin", 3, new[] { 0.0, 0.0, 0.0 });
        var xDir = GetDoubleArray(payload, "x_direction", 3, new[] { 1.0, 0.0, 0.0 });
        var yDir = GetDoubleArray(payload, "y_direction", 3, new[] { 0.0, 1.0, 0.0 });
        if (xDir[0] == 0 && xDir[1] == 0 && xDir[2] == 0) xDir[0] = 1.0;
        if (yDir[0] == 0 && yDir[1] == 0 && yDir[2] == 0) yDir[1] = 1.0;

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            var originPt = new Point3d(origin[0], origin[1], origin[2]);
            var xVec = new Vector3d(xDir[0], xDir[1], xDir[2]);
            var yVec = new Vector3d(yDir[0], yDir[1], yDir[2]);

            using (var scope = new BuilderScope<NXOpen.Features.DatumCsysBuilder>(
                part.Features.CreateDatumCsysBuilder(null!),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                var csys = part.CoordinateSystems.CreateCoordinateSystem(originPt, xVec, yVec);
                builder.Csys = csys;
                var feature = scope.CommitOnce(b => (NXOpen.Features.DatumCsys)b.CommitFeature());

                var csysHandle = Registry.Register(csys, "CoordinateSystem", ownerObjectId: partHandle.ObjectId);
                var featHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);

                return FormatResponse(requestId, new Dictionary<string, object>
                {
                    ["csys_ref"] = FormatHandle(csysHandle, csys),
                    ["feature_ref"] = FormatHandle(featHandle, feature),
                    ["name"] = feature.GetFeatureName(),
                });
            }
        }, token));
    }

    private static Task<byte[]> StartSketchCreate(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var name = GetString(payload, "name", "SKETCH");
        ObjectHandleToken? planeHandle = null;
        if (payload.TryGetValue("plane_ref", out var pRef) && pRef is Dictionary<string, object> pRefDict && pRefDict.Count > 0)
        {
            planeHandle = RequireHandle(new Dictionary<string, object> { ["plane"] = pRefDict }, "plane", "DatumPlane");
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            using (var scope = new BuilderScope<NXOpen.SketchInPlaceBuilder>(
                part.Sketches.CreateNewSketchInPlaceBuilder(null!),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                if (planeHandle != null)
                {
                    var datum = (DatumPlane)Registry.Resolve(planeHandle, "DatumPlane");
                    builder.PlaneReference = part.Planes.CreatePlane(datum.Origin, datum.Normal, NXOpen.SmartObject.UpdateOption.WithinModeling);
                }
                else
                {
                    builder.PlaneReference = part.Planes.CreatePlane(new Point3d(0, 0, 0), new Vector3d(0, 0, 1), NXOpen.SmartObject.UpdateOption.WithinModeling);
                }
                builder.SketchOrigin = part.Points.CreatePoint(new Point3d(0, 0, 0));

                var sketch = scope.CommitOnce(b => (NXOpen.Sketch)b.Commit());
                if (sketch == null) throw new InvalidOperationException("failed to commit sketch in place");

                if (!string.IsNullOrWhiteSpace(name))
                {
                    try { sketch.SetName(name); } catch { }
                }

                var sketchHandle = Registry.Register(sketch, "Sketch", ownerObjectId: partHandle.ObjectId);
                var feat = sketch.Feature;
                var featHandle = feat != null ? Registry.Register(feat, "Feature", ownerObjectId: partHandle.ObjectId) : null;

                return FormatResponse(requestId, new Dictionary<string, object>
                {
                    ["sketch_ref"] = FormatHandle(sketchHandle, sketch),
                    ["feature_ref"] = featHandle != null ? FormatHandle(featHandle, feat!) : new Dictionary<string, object>(),
                    ["name"] = sketch.Name ?? name,
                });
            }
        }, token));
    }

    private static Task<byte[]> StartSketchAddGeometry(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var sketchHandle = RequireHandle(payload, "sketch_ref", "Sketch");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var sketch = (NXOpen.Sketch)Registry.Resolve(sketchHandle, "Sketch");
            Journal.MarkStarted(requestId);

            var part = (Part)sketch.OwningPart;
            sketch.Activate(NXOpen.Sketch.ViewReorient.False);
            int addedCount = 0;

            try
            {
                // Lines
                if (payload.TryGetValue("lines", out var linesRaw) && linesRaw is IEnumerable<object> linesList)
                {
                    foreach (var item in linesList)
                    {
                        if (item is Dictionary<string, object> lineDict)
                        {
                            var start = GetDoubleArray(lineDict, "start", 2, new[] { 0.0, 0.0 });
                            var end = GetDoubleArray(lineDict, "end", 2, new[] { 0.0, 0.0 });
                            var line = part.Curves.CreateLine(new Point3d(start[0], start[1], 0), new Point3d(end[0], end[1], 0));
                            sketch.AddGeometry(line, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            addedCount++;
                        }
                    }
                }

                // Circles
                if (payload.TryGetValue("circles", out var circlesRaw) && circlesRaw is IEnumerable<object> circlesList)
                {
                    foreach (var item in circlesList)
                    {
                        if (item is Dictionary<string, object> circleDict)
                        {
                            var center = GetDoubleArray(circleDict, "center", 2, new[] { 0.0, 0.0 });
                            var radius = GetDouble(circleDict, "radius", 10.0);
                            var circle = part.Curves.CreateArc(
                                new Point3d(center[0], center[1], 0),
                                new Vector3d(1, 0, 0),
                                new Vector3d(0, 1, 0),
                                radius, 0, 2 * Math.PI);
                            sketch.AddGeometry(circle, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            addedCount++;
                        }
                    }
                }

                // Arcs
                if (payload.TryGetValue("arcs", out var arcsRaw) && arcsRaw is IEnumerable<object> arcsList)
                {
                    foreach (var item in arcsList)
                    {
                        if (item is Dictionary<string, object> arcDict)
                        {
                            var center = GetDoubleArray(arcDict, "center", 2, new[] { 0.0, 0.0 });
                            var radius = GetDouble(arcDict, "radius", 10.0);
                            var startAngle = GetDouble(arcDict, "start_angle", 0.0);
                            var endAngle = GetDouble(arcDict, "end_angle", Math.PI);
                            var arc = part.Curves.CreateArc(
                                new Point3d(center[0], center[1], 0),
                                new Vector3d(1, 0, 0),
                                new Vector3d(0, 1, 0),
                                radius, startAngle, endAngle);
                            sketch.AddGeometry(arc, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            addedCount++;
                        }
                    }
                }

                // Rectangles
                if (payload.TryGetValue("rectangles", out var rectsRaw) && rectsRaw is IEnumerable<object> rectsList)
                {
                    foreach (var item in rectsList)
                    {
                        if (item is Dictionary<string, object> rectDict)
                        {
                            var origin = GetDoubleArray(rectDict, "origin", 2, new[] { 0.0, 0.0 });
                            var width = GetDouble(rectDict, "width", 100.0);
                            var height = GetDouble(rectDict, "height", 100.0);

                            var p1 = new Point3d(origin[0], origin[1], 0);
                            var p2 = new Point3d(origin[0] + width, origin[1], 0);
                            var p3 = new Point3d(origin[0] + width, origin[1] + height, 0);
                            var p4 = new Point3d(origin[0], origin[1] + height, 0);

                            var l1 = part.Curves.CreateLine(p1, p2);
                            var l2 = part.Curves.CreateLine(p2, p3);
                            var l3 = part.Curves.CreateLine(p3, p4);
                            var l4 = part.Curves.CreateLine(p4, p1);

                            sketch.AddGeometry(l1, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            sketch.AddGeometry(l2, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            sketch.AddGeometry(l3, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            sketch.AddGeometry(l4, NXOpen.Sketch.InferConstraintsOption.InferCoincidentConstraints);
                            addedCount += 4;
                        }
                    }
                }

                sketch.Update();
            }
            finally
            {
                sketch.Deactivate(NXOpen.Sketch.ViewReorient.False, NXOpen.Sketch.UpdateLevel.Model);
            }

            var totalCurves = sketch.GetAllGeometry()?.Length ?? addedCount;
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["added_count"] = addedCount,
                ["curve_count"] = totalCurves,
            });
        }, token));
    }

    private static Task<byte[]> StartSketchQueryStatus(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var sketchHandle = RequireHandle(payload, "sketch_ref", "Sketch");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var sketch = (NXOpen.Sketch)Registry.Resolve(sketchHandle, "Sketch");

            int dofNeeded = 0;
            var status = sketch.GetStatus(out dofNeeded);
            var curves = sketch.GetAllGeometry()?.Length ?? 0;

            string statusStr;
            switch (status)
            {
                case NXOpen.Sketch.Status.UnderConstrained:
                    statusStr = "under_constrained";
                    break;
                case NXOpen.Sketch.Status.WellConstrained:
                    statusStr = "well_constrained";
                    break;
                case NXOpen.Sketch.Status.OverConstrained:
                    statusStr = "over_constrained";
                    break;
                case NXOpen.Sketch.Status.InconsistentlyConstrained:
                    statusStr = "inconsistently_constrained";
                    break;
                default:
                    statusStr = status.ToString().ToLowerInvariant();
                    break;
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["status"] = statusStr,
                ["dof_needed"] = dofNeeded,
                ["curve_count"] = curves,
            });
        }, token));
    }

    private static Task<byte[]> StartProfileCreate(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var sketchHandle = RequireHandle(payload, "sketch_ref", "Sketch");
        var chainTol = GetDouble(payload, "chaining_tolerance", 0.024);
        var distTol = GetDouble(payload, "distance_tolerance", 0.024);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var sketch = (NXOpen.Sketch)Registry.Resolve(sketchHandle, "Sketch");
            Journal.MarkStarted(requestId);

            var section = part.Sections.CreateSection(
                chainTol > 0 ? chainTol : 0.024,
                distTol > 0 ? distTol : 0.024,
                0.5);

            if (sketch.Feature != null)
            {
                var curveRule = part.ScRuleFactory.CreateRuleCurveFeature(new NXOpen.Features.Feature[] { sketch.Feature });
                section.AddToSection(
                    new NXOpen.SelectionIntentRule[] { curveRule },
                    null!, null!, null!,
                    new Point3d(0, 0, 0),
                    NXOpen.Section.Mode.Create, false);
            }
            else
            {
                var allGeom = sketch.GetAllGeometry();
                var curves = new List<NXOpen.Curve>();
                if (allGeom != null)
                {
                    foreach (var g in allGeom)
                    {
                        if (g is NXOpen.Curve c) curves.Add(c);
                    }
                }
                if (curves.Count > 0)
                {
                    var dumbRule = part.ScRuleFactory.CreateRuleCurveDumb(curves.ToArray());
                    section.AddToSection(
                        new NXOpen.SelectionIntentRule[] { dumbRule },
                        null!, null!, null!,
                        new Point3d(0, 0, 0),
                        NXOpen.Section.Mode.Create, false);
                }
            }

            int loopCount = 0;
            try { loopCount = section.GetNumberOfLoops(); } catch { }

            var profHandle = Registry.Register(section, "Profile", ownerObjectId: partHandle.ObjectId);

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["profile_ref"] = FormatHandle(profHandle, section),
                ["name"] = "Profile",
                ["loop_count"] = loopCount,
            });
        }, token));
    }

    private static Task<byte[]> StartCreateExtrude(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var startLimit = GetDouble(payload, "start_limit", 0.0);
        var endLimit = GetDouble(payload, "end_limit", 25.0);

        ObjectHandleToken? profileHandle = null;
        if (payload.TryGetValue("profile_ref", out var profRaw) && profRaw is Dictionary<string, object> profDict && profDict.Count > 0)
        {
            profileHandle = RequireHandle(new Dictionary<string, object> { ["prof"] = profDict }, "prof", "Profile");
        }
        ObjectHandleToken? sketchHandle = null;
        if (payload.TryGetValue("sketch_ref", out var skRaw) && skRaw is Dictionary<string, object> skDict && skDict.Count > 0)
        {
            sketchHandle = RequireHandle(new Dictionary<string, object> { ["sketch"] = skDict }, "sketch", "Sketch");
        }
        if (profileHandle == null && sketchHandle == null)
        {
            throw new ArgumentException("either profile_ref or sketch_ref is required for extrude");
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            NXOpen.Section section;
            if (profileHandle != null)
            {
                section = (NXOpen.Section)Registry.Resolve(profileHandle, "Profile");
            }
            else
            {
                var sketch = (NXOpen.Sketch)Registry.Resolve(sketchHandle!, "Sketch");
                section = part.Sections.CreateSection(0.024, 0.024, 0.5);
                if (sketch.Feature != null)
                {
                    var rule = part.ScRuleFactory.CreateRuleCurveFeature(new NXOpen.Features.Feature[] { sketch.Feature });
                    section.AddToSection(new NXOpen.SelectionIntentRule[] { rule }, null!, null!, null!, new Point3d(0, 0, 0), NXOpen.Section.Mode.Create, false);
                }
                else
                {
                    var allGeom = sketch.GetAllGeometry();
                    var curves = new List<NXOpen.Curve>();
                    if (allGeom != null)
                    {
                        foreach (var g in allGeom)
                        {
                            if (g is NXOpen.Curve c) curves.Add(c);
                        }
                    }
                    if (curves.Count > 0)
                    {
                        var dumbRule = part.ScRuleFactory.CreateRuleCurveDumb(curves.ToArray());
                        section.AddToSection(new NXOpen.SelectionIntentRule[] { dumbRule }, null!, null!, null!, new Point3d(0, 0, 0), NXOpen.Section.Mode.Create, false);
                    }
                }
            }

            using (var scope = new BuilderScope<NXOpen.Features.ExtrudeBuilder>(
                part.Features.CreateExtrudeBuilder(null!),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Section = section;

                if (payload.TryGetValue("direction", out var dirRaw) && dirRaw != null)
                {
                    var dir = GetDoubleArray(payload, "direction", 3, new[] { 0.0, 0.0, 1.0 });
                    if (dir[0] != 0 || dir[1] != 0 || dir[2] != 0)
                    {
                        builder.Direction = part.Directions.CreateDirection(new Point3d(0, 0, 0), new Vector3d(dir[0], dir[1], dir[2]), NXOpen.SmartObject.UpdateOption.WithinModeling);
                    }
                }

                builder.Limits.StartExtend.Value.RightHandSide = startLimit.ToString("G", CultureInfo.InvariantCulture);
                builder.Limits.EndExtend.Value.RightHandSide = endLimit.ToString("G", CultureInfo.InvariantCulture);

                ApplyBooleanOption(part, builder.BooleanOperation, payload);

                var feature = scope.CommitOnce(b => (NXOpen.Features.BodyFeature)b.CommitFeature());
                var bodies = feature.GetBodies();
                if (bodies == null || bodies.Length == 0) throw new InvalidOperationException("extrude feature commit produced no body");
                var body = bodies[0];

                var featureHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);
                var bodyHandle = Registry.Register(body, "Body", ownerObjectId: partHandle.ObjectId);

                return FormatResponse(requestId, new Dictionary<string, object>
                {
                    ["feature_ref"] = FormatHandle(featureHandle, feature),
                    ["body_ref"] = FormatHandle(bodyHandle, body),
                    ["feature_name"] = feature.GetFeatureName(),
                    ["feature_type"] = feature.FeatureType,
                });
            }
        }, token));
    }

    private static Task<byte[]> StartCreateRevolve(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var startAngle = GetDouble(payload, "start_angle", 0.0);
        var endAngle = GetDouble(payload, "end_angle", 360.0);

        ObjectHandleToken? profileHandle = null;
        if (payload.TryGetValue("profile_ref", out var profRaw) && profRaw is Dictionary<string, object> profDict && profDict.Count > 0)
        {
            profileHandle = RequireHandle(new Dictionary<string, object> { ["prof"] = profDict }, "prof", "Profile");
        }
        ObjectHandleToken? sketchHandle = null;
        if (payload.TryGetValue("sketch_ref", out var skRaw) && skRaw is Dictionary<string, object> skDict && skDict.Count > 0)
        {
            sketchHandle = RequireHandle(new Dictionary<string, object> { ["sketch"] = skDict }, "sketch", "Sketch");
        }
        if (profileHandle == null && sketchHandle == null)
        {
            throw new ArgumentException("either profile_ref or sketch_ref is required for revolve");
        }

        ObjectHandleToken? axisHandle = null;
        if (payload.TryGetValue("axis_ref", out var axRaw) && axRaw is Dictionary<string, object> axDict && axDict.Count > 0)
        {
            axisHandle = RequireHandle(new Dictionary<string, object> { ["axis"] = axDict }, "axis", "DatumAxis");
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            NXOpen.Section section;
            if (profileHandle != null)
            {
                section = (NXOpen.Section)Registry.Resolve(profileHandle, "Profile");
            }
            else
            {
                var sketch = (NXOpen.Sketch)Registry.Resolve(sketchHandle!, "Sketch");
                section = part.Sections.CreateSection(0.024, 0.024, 0.5);
                if (sketch.Feature != null)
                {
                    var rule = part.ScRuleFactory.CreateRuleCurveFeature(new NXOpen.Features.Feature[] { sketch.Feature });
                    section.AddToSection(new NXOpen.SelectionIntentRule[] { rule }, null!, null!, null!, new Point3d(0, 0, 0), NXOpen.Section.Mode.Create, false);
                }
                else
                {
                    var allGeom = sketch.GetAllGeometry();
                    var curves = new List<NXOpen.Curve>();
                    if (allGeom != null)
                    {
                        foreach (var g in allGeom)
                        {
                            if (g is NXOpen.Curve c) curves.Add(c);
                        }
                    }
                    if (curves.Count > 0)
                    {
                        var dumbRule = part.ScRuleFactory.CreateRuleCurveDumb(curves.ToArray());
                        section.AddToSection(new NXOpen.SelectionIntentRule[] { dumbRule }, null!, null!, null!, new Point3d(0, 0, 0), NXOpen.Section.Mode.Create, false);
                    }
                }
            }

            using (var scope = new BuilderScope<NXOpen.Features.RevolveBuilder>(
                part.Features.CreateRevolveBuilder(null!),
                b => { try { b.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Section = section;

                if (axisHandle != null)
                {
                    var datumAxis = (DatumAxis)Registry.Resolve(axisHandle, "DatumAxis");
                    var startPt = datumAxis.Origin;
                    var dirVec = datumAxis.Direction;
                    builder.Axis = part.Axes.CreateAxis(startPt, dirVec, NXOpen.SmartObject.UpdateOption.WithinModeling);
                }
                else
                {
                    var origin = GetDoubleArray(payload, "axis_origin", 3, new[] { 0.0, 0.0, 0.0 });
                    var dir = GetDoubleArray(payload, "axis_direction", 3, new[] { 0.0, 0.0, 1.0 });
                    if (dir[0] == 0 && dir[1] == 0 && dir[2] == 0) dir[2] = 1.0;
                    builder.Axis = part.Axes.CreateAxis(
                        new Point3d(origin[0], origin[1], origin[2]),
                        new Vector3d(dir[0], dir[1], dir[2]),
                        NXOpen.SmartObject.UpdateOption.WithinModeling);
                }

                builder.Limits.StartExtend.Value.RightHandSide = startAngle.ToString("G", CultureInfo.InvariantCulture);
                builder.Limits.EndExtend.Value.RightHandSide = endAngle.ToString("G", CultureInfo.InvariantCulture);

                ApplyBooleanOption(part, builder.BooleanOperation, payload);

                var feature = scope.CommitOnce(b => (NXOpen.Features.BodyFeature)b.CommitFeature());
                var bodies = feature.GetBodies();
                if (bodies == null || bodies.Length == 0) throw new InvalidOperationException("revolve feature commit produced no body");
                var body = bodies[0];

                var featureHandle = Registry.Register(feature, "Feature", ownerObjectId: partHandle.ObjectId);
                var bodyHandle = Registry.Register(body, "Body", ownerObjectId: partHandle.ObjectId);

                return FormatResponse(requestId, new Dictionary<string, object>
                {
                    ["feature_ref"] = FormatHandle(featureHandle, feature),
                    ["body_ref"] = FormatHandle(bodyHandle, body),
                    ["feature_name"] = feature.GetFeatureName(),
                    ["feature_type"] = feature.FeatureType,
                });
            }
        }, token));
    }

    private static double GetDouble(Dictionary<string, object> source, string key, double defaultValue)
    {
        if (!source.TryGetValue(key, out var value) || value == null) return defaultValue;
        return Convert.ToDouble(value, CultureInfo.InvariantCulture);
    }

    private static double[] GetDoubleArray(Dictionary<string, object> source, string key, int requiredLength, double[] defaultValue)
    {
        object? raw;
        if (!source.TryGetValue(key, out raw) || raw == null) return (double[])defaultValue.Clone();
        var items = ToObjectArray(raw);
        if (items.Length != requiredLength) throw new ArgumentException(key + " must contain exactly " + requiredLength + " numbers");
        var result = new double[requiredLength];
        for (var i = 0; i < requiredLength; i++) result[i] = Convert.ToDouble(items[i], CultureInfo.InvariantCulture);
        return result;
    }

    private static object[] GetArray(Dictionary<string, object> source, string key)
    {
        object? raw;
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


