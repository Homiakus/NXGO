using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using NXGO.Agent.Core;
using NXOpen;

namespace NXGO.Agent.NXHost;

/// <summary>
/// Dedicated-worker bootstrap for the canonical compiled Agent path.
/// Transport threads only parse/enqueue. All NXOpen work executes through the
/// shared Agent.Core NxExecutor on the bound NX execution thread.
/// </summary>
public static partial class EntryPoint
{
    private const int ProtocolMajor = 2;
    private const int ProtocolMinor = 0;
    private const ulong Epoch = 1;
    private static readonly SessionHealthState Health = new SessionHealthState();
    private static readonly string SessionId = "nxgo-" + Guid.NewGuid().ToString("N");
    private static readonly HandleRegistry<TaggedObject> Registry = new HandleRegistry<TaggedObject>(SessionId, Epoch, 4096);
    private static readonly RequestJournal Journal = new RequestJournal(RequestJournal.DefaultCapacity);
    private static readonly JavaScriptSerializer Json = new JavaScriptSerializer { MaxJsonLength = 4 * 1024 * 1024 };
    private static volatile bool _shutdownRequested;

    public static void Main(string[] args)
    {
        var session = Session.GetSession();
        var executor = new NxExecutor();
        executor.BindToCurrentThread();

        var pipeName = Environment.GetEnvironmentVariable("NXGO_PIPE_NAME");
        if (string.IsNullOrWhiteSpace(pipeName))
        {
            pipeName = "nxgo-worker-" + Process.GetCurrentProcess().Id;
        }

        using (var server = new NamedPipeRequestServer(pipeName!, (payload, token) => HandleRequest(session, executor, payload, token)))
        {
            session.LogFile.WriteLine($"[NXGO] canonical NXHost start pipe={pipeName} protocol={ProtocolMajor}.{ProtocolMinor}");
            server.Start();

            while (!_shutdownRequested && Health.Value != SessionHealth.Lost)
            {
                executor.DrainUntilEmpty(64);
                Thread.Sleep(5);
            }

            session.LogFile.WriteLine($"[NXGO] canonical NXHost stop health={Health.Value} registry={Registry.Count}/{Registry.Capacity} high_water={Registry.HighWatermark} journal={Journal.Count}/{Journal.Capacity}");
        }
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }

    private static Task<byte[]> HandleRequest(Session session, NxExecutor executor, byte[] payload, CancellationToken token)
    {
        Dictionary<string, object> envelope;
        try
        {
            envelope = DecodeObject(payload);
        }
        catch (Exception ex)
        {
            return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid JSON request: " + ex.Message, false));
        }

        if (envelope.ContainsKey("protocol_version") && envelope.ContainsKey("nonce") && !envelope.ContainsKey("request_id"))
        {
            return Task.FromResult(FormatHandshake(session));
        }

        var requestId = GetString(envelope, "request_id", string.Empty);
        var operation = GetString(envelope, "op", string.Empty);
        if (string.IsNullOrWhiteSpace(requestId) || string.IsNullOrWhiteSpace(operation))
        {
            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", "request_id and op are required", false));
        }
        var requestPayload = GetObject(envelope, "payload", required: false) ?? new Dictionary<string, object>(StringComparer.Ordinal);

        if (operation == "shutdown")
        {
            _shutdownRequested = true;
            return Task.FromResult(FormatResponse(requestId, new Dictionary<string, object> { ["shutdown"] = true }));
        }

        if (IsJournaledMutation(operation))
        {
            RequestAdmission admission;
            try
            {
                admission = Journal.Admit(requestId, operation, Encoding.UTF8.GetBytes(Json.Serialize(requestPayload)));
            }
            catch (RequestIdentityConflictException ex)
            {
                Health.MarkPoisoned();
                return Task.FromResult(FormatError(requestId, "REQUEST_IDENTITY_CONFLICT", ex.Message, false));
            }
            catch (RequestJournalCapacityException ex)
            {
                Health.MarkPoisoned();
                return Task.FromResult(FormatError(requestId, "JOURNAL_CAPACITY", ex.Message, false));
            }

            switch (admission.Disposition)
            {
                case RequestReplayDisposition.ReturnCommittedResult:
                case RequestReplayDisposition.ReturnRolledBackResult:
                case RequestReplayDisposition.ReturnFailure:
                    if (admission.Record.ResultEnvelope != null)
                    {
                        return Task.FromResult((byte[])admission.Record.ResultEnvelope.Clone());
                    }
                    return Task.FromResult(FormatError(requestId, "JOURNAL_REPLAY_ERROR", admission.Record.Failure ?? "journal replay has no result envelope", false));
                case RequestReplayDisposition.InFlight:
                    return Task.FromResult(FormatError(requestId, "REQUEST_IN_FLIGHT", "request with this request_id is already executing", true));
                case RequestReplayDisposition.OutcomeUnknown:
                    return Task.FromResult(FormatError(requestId, "OUTCOME_UNKNOWN", "previous execution outcome is unknown; request must not be replayed", false));
                case RequestReplayDisposition.New:
                    break;
                default:
                    Health.MarkPoisoned();
                    return Task.FromResult(FormatError(requestId, "JOURNAL_ERROR", "unknown journal replay disposition", false));
            }
        }

        try
        {
            switch (operation)
            {
                case "nx.ping":
                    return MapRead(requestId, executor.EnqueueTracked(() =>
                    {
                        Health.RequireReusable();
                        session.LogFile.WriteLine("[NXGO] canonical nx.ping");
                        return FormatResponse(requestId, new Dictionary<string, object> { ["ping"] = "pong" });
                    }, token));

                case "session.info":
                    return MapRead(requestId, executor.EnqueueTracked(() =>
                    {
                        Health.RequireReusable();
                        var release = session.GetEnvironmentVariableValue("UGII_VERSION") ?? string.Empty;
                        var baseDir = session.GetEnvironmentVariableValue("UGII_BASE_DIR") ?? string.Empty;
                        return FormatResponse(requestId, new Dictionary<string, object>
                        {
                            ["release"] = release,
                            ["base_dir"] = baseDir,
                            ["thread_id"] = Environment.CurrentManagedThreadId,
                            ["epoch"] = Epoch,
                            ["session_id"] = SessionId,
                        });
                    }, token));

                case "part.new":
                    return StartPartNew(session, executor, requestId, requestPayload, token);
                case "part.open":
                    return StartPartOpen(session, executor, requestId, requestPayload, token);
                case "part.save":
                    return StartPartSave(executor, requestId, requestPayload, token);
                case "part.close":
                    return StartPartClose(executor, requestId, requestPayload, token);
                case "part.query_summary":
                    return StartPartSummary(executor, requestId, requestPayload, token);
                case "object.release":
                    return StartObjectRelease(executor, requestId, requestPayload, token);
                case "feature.create_block":
                    return StartCreateBlock(executor, requestId, requestPayload, token);
                case "feature.create_cylinder":
                    return StartCreateCylinder(executor, requestId, requestPayload, token);
                case "part.query_bodies":
                    return StartQueryBodies(executor, requestId, requestPayload, token);
                case "geometry.query_mass_properties":
                    return StartMassProperties(executor, requestId, requestPayload, token);
                case "geometry.query_bounding_box":
                    return StartBoundingBox(executor, requestId, requestPayload, token);

                default:
                    return Task.FromResult(FormatError(requestId, "UNSUPPORTED_OPERATION", "canonical NXHost operation is not migrated yet: " + operation, false));
            }
        }
        catch (StaleObjectHandleException ex)
        {
            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false));
        }
        catch (ArgumentException ex)
        {
            if (IsJournaledMutation(operation))
            {
                var response = FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);
                TryMarkFailedBeforeStart(requestId, ex.Message, response);
                return Task.FromResult(response);
            }
            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false));
        }
        catch (Exception ex)
        {
            Health.MarkSuspect();
            if (IsJournaledMutation(operation))
            {
                var response = FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false);
                TryMarkFailedBeforeStart(requestId, ex.Message, response);
                return Task.FromResult(response);
            }
            return Task.FromResult(FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false));
        }
    }

    private static Task<byte[]> StartPartNew(Session session, NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var partName = GetString(payload, "name", string.Empty);
        if (string.IsNullOrWhiteSpace(partName))
        {
            partName = "model_" + Guid.NewGuid().ToString("N").Substring(0, 6) + ".prt";
        }
        var unitsText = GetString(payload, "units", "mm");
        var units = string.Equals(unitsText, "in", StringComparison.OrdinalIgnoreCase) ||
                    string.Equals(unitsText, "inches", StringComparison.OrdinalIgnoreCase)
            ? Part.Units.Inches
            : Part.Units.Millimeters;

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId);
            var part = session.Parts.NewDisplay(partName, units);
            var handle = Registry.Register(part, "Part");
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["part_ref"] = FormatHandle(handle, part),
                ["name"] = part.Name,
                ["units"] = part.PartUnits.ToString(),
            });
        }, token));
    }

    private static Task<byte[]> StartPartOpen(Session session, NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var path = GetString(payload, "path", string.Empty);
        if (string.IsNullOrWhiteSpace(path)) throw new ArgumentException("missing part path for part.open");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId);
            PartLoadStatus loadStatus;
            var part = session.Parts.OpenDisplay(path, out loadStatus);
            if (loadStatus != null) loadStatus.Dispose();
            var handle = Registry.Register(part, "Part");
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["part_ref"] = FormatHandle(handle, part),
                ["name"] = part.Name,
                ["units"] = part.PartUnits.ToString(),
            });
        }, token));
    }

    private static Task<byte[]> StartPartSave(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");
        var part = (Part)Registry.Resolve(handle, "Part");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId);
            part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["saved"] = true,
                ["name"] = part.Name,
                ["full_path"] = (part.FullPath ?? string.Empty).Replace('\\', '/'),
            });
        }, token));
    }

    private static Task<byte[]> StartPartClose(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");
        var part = (Part)Registry.Resolve(handle, "Part");
        var save = GetBool(payload, "save", false);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId);
            var name = part.Name;
            if (save)
            {
                part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);
            }
            part.Close(BasePart.CloseWholeTree.False, BasePart.CloseModified.CloseModified, null);
            Registry.ReleaseWithDependents(handle);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["closed"] = true,
                ["name"] = name,
            });
        }, token));
    }

    private static Task<byte[]> StartPartSummary(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");
        var part = (Part)Registry.Resolve(handle, "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var bodyCount = 0;
            foreach (Body _ in part.Bodies) bodyCount++;
            var featureCount = 0;
            foreach (NXOpen.Features.Feature _ in part.Features) featureCount++;
            var componentCount = 0;
            if (part.ComponentAssembly != null && part.ComponentAssembly.RootComponent != null)
            {
                componentCount = part.ComponentAssembly.RootComponent.GetChildren().Length;
            }
            uint nativeTag = 0;
            try { nativeTag = (uint)part.Tag; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["name"] = part.Name,
                ["units"] = part.PartUnits.ToString(),
                ["body_count"] = bodyCount,
                ["feature_count"] = featureCount,
                ["component_count"] = componentCount,
                ["native_tag"] = nativeTag,
            });
        }, token));
    }

    private static async Task<byte[]> MapRead(string requestId, NxExecution<byte[]> execution)
    {
        try
        {
            return await execution.Task.ConfigureAwait(false);
        }
        catch (TaskCanceledException ex)
        {
            return FormatError(requestId, "CANCELLED", ex.Message, true);
        }
        catch (Exception ex)
        {
            Health.MarkSuspect();
            return FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false);
        }
    }

    private static async Task<byte[]> MapMutation(string requestId, NxExecution<byte[]> execution)
    {
        try
        {
            var response = await execution.Task.ConfigureAwait(false);
            Journal.MarkCommitted(requestId, response);
            return response;
        }
        catch (TaskCanceledException ex)
        {
            var response = FormatError(requestId, "CANCELLED_BEFORE_START", ex.Message, true);
            TryMarkFailedBeforeStart(requestId, ex.Message, response);
            return response;
        }
        catch (Exception ex)
        {
            RequestJournalRecord? record;
            if (Journal.TryGet(requestId, out record) && record != null && record.State == RequestJournalState.Started)
            {
                Journal.MarkOutcomeUnknown(requestId, ex.GetType().Name + ": " + ex.Message);
                Health.MarkLost();
                return FormatError(requestId, "OUTCOME_UNKNOWN", "NX mutation faulted after execution started; worker is quarantined: " + ex.Message, false);
            }

            var response = FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false);
            TryMarkFailedBeforeStart(requestId, ex.Message, response);
            Health.MarkSuspect();
            return response;
        }
    }

    private static void TryMarkFailedBeforeStart(string requestId, string diagnostic, byte[] response)
    {
        RequestJournalRecord? record;
        if (Journal.TryGet(requestId, out record) && record != null && record.State == RequestJournalState.Received)
        {
            Journal.MarkFailed(requestId, string.IsNullOrWhiteSpace(diagnostic) ? "request failed before NX execution" : diagnostic, response);
        }
    }

    private static bool IsJournaledMutation(string operation)
    {
        switch (operation)
        {
            case "part.new":
            case "part.open":
            case "part.save":
            case "part.close":
            case "object.release":
            case "feature.create_block":
            case "feature.create_cylinder":
                return true;
            default:
                return false;
        }
    }

    private static byte[] FormatHandshake(Session session)
    {
        var release = session.GetEnvironmentVariableValue("UGII_VERSION");
        if (string.IsNullOrWhiteSpace(release)) release = "unknown";
        return Serialize(new Dictionary<string, object>
        {
            ["protocol_version"] = new Dictionary<string, object>
            {
                ["major"] = ProtocolMajor,
                ["minor"] = ProtocolMinor,
            },
            ["agent_version"] = "v0.2.0-nxhost",
            ["nx_release"] = release,
            ["nx_build"] = release + ".compiled",
            ["nx_pid"] = Process.GetCurrentProcess().Id,
            ["session_id"] = SessionId,
            ["epoch"] = Epoch,
            ["capabilities"] = new[]
            {
                "nx.ping",
                "session.info",
                "part.new",
                "part.open",
                "part.save",
                "part.close",
                "part.query_summary",
                "object.release",
                "feature.create_block",
                "feature.create_cylinder",
                "part.query_bodies",
                "geometry.query_mass_properties",
                "geometry.query_bounding_box",
                "shutdown",
            },
            ["max_payload_bytes"] = 4 * 1024 * 1024,
            ["security_policy"] = "local_pipe_only",
        });
    }

    private static byte[] FormatResponse(string requestId, object payload)
    {
        return Serialize(new Dictionary<string, object>
        {
            ["request_id"] = requestId ?? string.Empty,
            ["status"] = "OK",
            ["payload"] = payload ?? new Dictionary<string, object>(),
        });
    }

    private static byte[] FormatError(string requestId, string category, string message, bool recoverable)
    {
        return Serialize(new Dictionary<string, object>
        {
            ["request_id"] = requestId ?? string.Empty,
            ["status"] = "ERROR",
            ["error"] = new Dictionary<string, object>
            {
                ["category"] = category,
                ["nx_error_code"] = 0,
                ["message"] = message ?? string.Empty,
                ["recoverable"] = recoverable,
                ["session_health"] = WireHealth(),
            },
        });
    }

    private static string WireHealth()
    {
        return Health.Value == SessionHealth.Healthy
            ? "healthy"
            : Health.Value == SessionHealth.Lost
                ? "lost"
                : "dirty";
    }

    private static byte[] Serialize(object value)
    {
        return Encoding.UTF8.GetBytes(Json.Serialize(value));
    }

    private static Dictionary<string, object> DecodeObject(byte[] payload)
    {
        var text = Encoding.UTF8.GetString(payload ?? Array.Empty<byte>());
        var decoded = Json.DeserializeObject(text) as Dictionary<string, object>;
        if (decoded == null) throw new ArgumentException("request must be a JSON object");
        return decoded;
    }

    private static Dictionary<string, object>? GetObject(Dictionary<string, object> source, string key, bool required)
    {
        object value;
        if (!source.TryGetValue(key, out value) || value == null)
        {
            if (required) throw new ArgumentException("missing object field: " + key);
            return null;
        }
        var result = value as Dictionary<string, object>;
        if (result == null) throw new ArgumentException("field must be an object: " + key);
        return result;
    }

    private static string GetString(Dictionary<string, object> source, string key, string defaultValue)
    {
        object value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        return Convert.ToString(value, CultureInfo.InvariantCulture) ?? defaultValue;
    }

    private static bool GetBool(Dictionary<string, object> source, string key, bool defaultValue)
    {
        object value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        if (value is bool) return (bool)value;
        bool parsed;
        return bool.TryParse(Convert.ToString(value, CultureInfo.InvariantCulture), out parsed) ? parsed : defaultValue;
    }

    private static ulong GetUInt64(Dictionary<string, object> source, string key)
    {
        object value;
        if (!source.TryGetValue(key, out value) || value == null) throw new ArgumentException("missing numeric field: " + key);
        return Convert.ToUInt64(value, CultureInfo.InvariantCulture);
    }

    private static uint GetUInt32(Dictionary<string, object> source, string key)
    {
        object value;
        if (!source.TryGetValue(key, out value) || value == null) throw new ArgumentException("missing numeric field: " + key);
        return Convert.ToUInt32(value, CultureInfo.InvariantCulture);
    }

    private static ObjectHandleToken RequireHandle(Dictionary<string, object> payload, string key, string expectedKind)
    {
        var raw = GetObject(payload, key, required: true)!;
        var token = new ObjectHandleToken
        {
            SessionId = GetString(raw, "session_id", string.Empty),
            Epoch = GetUInt64(raw, "epoch"),
            ObjectId = GetString(raw, "object_id", string.Empty),
            Generation = GetUInt32(raw, "generation"),
            Kind = GetString(raw, "kind", string.Empty),
            LeaseScopeId = GetString(raw, "lease_scope_id", string.Empty),
        };
        if (!string.Equals(token.Kind, expectedKind, StringComparison.OrdinalIgnoreCase))
        {
            throw new StaleObjectHandleException($"wrong object kind for {token.ObjectId}: got {token.Kind}, expected {expectedKind}");
        }
        return token;
    }

    private static Dictionary<string, object> FormatHandle(ObjectHandleToken token, TaggedObject target)
    {
        uint nativeTag = 0;
        try { nativeTag = (uint)target.Tag; } catch { }
        return new Dictionary<string, object>
        {
            ["session_id"] = token.SessionId,
            ["epoch"] = token.Epoch,
            ["object_id"] = token.ObjectId,
            ["generation"] = token.Generation,
            ["kind"] = token.Kind,
            ["native_tag"] = nativeTag,
            ["lease_scope_id"] = token.LeaseScopeId,
        };
    }
}
