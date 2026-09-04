using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXGO.Protocol;
using NXOpen;

namespace NXGO.Agent.NXHost;

/// <summary>
/// Dedicated-worker bootstrap for the canonical compiled Agent path.
/// Transport threads only parse/enqueue. All NXOpen work executes through the
/// shared Agent.Core NxExecutor on the bound NX execution thread.
/// </summary>
public static partial class EntryPoint
{
    private enum MutationOutcomeClass { DeterministicIdempotent, Transactional, AmbiguousNonRetryable }
    private const int ProtocolMajor = 2;
    private const int ProtocolMinor = 0;
    private const ulong Epoch = 1;
    private static readonly SessionHealthState Health = new SessionHealthState();
    private static readonly string SessionId = "nxgo-" + Guid.NewGuid().ToString("N");
    private static readonly HandleRegistry<TaggedObject> Registry = new HandleRegistry<TaggedObject>(SessionId, Epoch, 4096);
    private static readonly AtomicRequestJournalStore? JournalStore = CreateJournalStore();
    private static readonly RequestJournal Journal = LoadJournal(JournalStore);
    private static readonly JsonWireCodec Wire = new JsonWireCodec(JsonWireCodec.DefaultMaxPayloadBytes);
    // The same registry drives handshake advertisement and request admission.
    private static readonly HashSet<string> SupportedOperations = new HashSet<string>(StringComparer.Ordinal)
    {
        "nx.ping", "session.info", "part.new", "part.open", "part.save", "part.close",
        "part.query_summary", "part.get_attributes", "part.set_attributes", "part.bulk_metadata",
        "part.query_load_status",
        "object.release", "feature.create_block", "feature.create_cylinder", "feature.boolean",
        "feature.create_hole",
        "part.query_bodies", "geometry.query_mass_properties", "geometry.query_bounding_box",
        "transaction.begin", "transaction.commit", "transaction.rollback", "assembly.add_component",
        "assembly.query_tree", "assembly.query_bom", "assembly.remove_component",
        "drafting.create_sheet", "drafting.query_sheets", "drafting.export_pdf", "shutdown",
    };
    private static readonly Dictionary<string, MutationOutcomeClass> MutationClasses = new Dictionary<string, MutationOutcomeClass>(StringComparer.Ordinal)
    {
        ["part.new"] = MutationOutcomeClass.Transactional, ["part.open"] = MutationOutcomeClass.Transactional,
        ["part.save"] = MutationOutcomeClass.Transactional, ["part.close"] = MutationOutcomeClass.Transactional,
        ["part.set_attributes"] = MutationOutcomeClass.Transactional,
        ["object.release"] = MutationOutcomeClass.DeterministicIdempotent,
        ["feature.create_block"] = MutationOutcomeClass.Transactional, ["feature.create_cylinder"] = MutationOutcomeClass.Transactional,
        ["feature.boolean"] = MutationOutcomeClass.Transactional, ["feature.create_hole"] = MutationOutcomeClass.Transactional,
        ["transaction.begin"] = MutationOutcomeClass.Transactional, ["transaction.commit"] = MutationOutcomeClass.Transactional,
        ["transaction.rollback"] = MutationOutcomeClass.Transactional,
        ["assembly.add_component"] = MutationOutcomeClass.Transactional, ["assembly.remove_component"] = MutationOutcomeClass.Transactional,
        ["drafting.create_sheet"] = MutationOutcomeClass.Transactional, ["drafting.export_pdf"] = MutationOutcomeClass.Transactional,
    };
    private static volatile bool _shutdownRequested;
    private static volatile bool _authenticated;
    private static readonly string? ExpectedNonce = Environment.GetEnvironmentVariable("NXGO_WORKER_NONCE");

    public static string RuntimeAssemblyIdentity()
    {
        return typeof(Session).Assembly.FullName + " @ " + typeof(Session).Assembly.Location +
            " | " + typeof(TaggedObject).Assembly.FullName + " @ " + typeof(TaggedObject).Assembly.Location;
    }

    // Deliberately not named Main: run_journal invokes the bootstrap Main and
    // may discover public Main methods in loaded assemblies as journal entry
    // points, passing NX objects with an incompatible signature.
    // Canonical entry point consumed by CompiledHostBootstrap: EntryPoint.Run.
    public static void Run(string[] args)
    {
        Program.WriteDiagnostic("NXGO: calling Session.GetSession()...");
        var session = Session.GetSession() ?? throw new InvalidOperationException("Session.GetSession() returned null");
        Program.WriteDiagnostic("NXGO: Session.GetSession() completed: ok");
        var executor = new NxExecutor();
        executor.BindToCurrentThread();

        var pipeName = Environment.GetEnvironmentVariable("NXGO_PIPE_NAME");
        if (string.IsNullOrWhiteSpace(pipeName))
        {
            pipeName = "nxgo-worker-" + Process.GetCurrentProcess().Id;
        }

        Program.WriteDiagnostic($"NXGO: starting pipe server on {pipeName}...");
        using (var server = new NamedPipeRequestServer(
            pipeName!,
            (payload, token) => HandleRequest(session, executor, payload, token),
            () => { _authenticated = false; }))
        {
            session.LogFile.WriteLine($"[NXGO] canonical NXHost start pipe={pipeName} protocol={ProtocolMajor}.{ProtocolMinor}");
            server.Start();
            Program.WriteDiagnostic($"NXGO: pipe server started on {pipeName}");

            while (!_shutdownRequested && Health.Value != SessionHealth.Lost)
            {
                executor.DrainUntilEmpty(64);
                Thread.Sleep(5);
            }

            SaveJournal(session);
            session.LogFile.WriteLine($"[NXGO] canonical NXHost stop health={Health.Value} registry={Registry.Count}/{Registry.Capacity} high_water={Registry.HighWatermark} journal={Journal.Count}/{Journal.Capacity}");
            Program.WriteDiagnostic("NXGO: server loop exited");
        }
    }

    private static AtomicRequestJournalStore? CreateJournalStore()
    {
        var path = Environment.GetEnvironmentVariable("NXGO_JOURNAL_STATE");
        return string.IsNullOrWhiteSpace(path) ? null : new AtomicRequestJournalStore(path);
    }

    private static RequestJournal LoadJournal(AtomicRequestJournalStore? store)
    {
        if (store == null || !File.Exists(store.Path)) return new RequestJournal(RequestJournal.DefaultCapacity);
        return store.Load(RequestJournal.DefaultCapacity);
    }

    private static void SaveJournal(Session session)
    {
        if (JournalStore == null) return;
        try { JournalStore.Save(Journal); }
        catch (Exception ex) { session.LogFile.WriteLine("[NXGO] journal persistence failed: " + ex.Message); }
    }

    private static void PersistJournalOrThrow()
    {
        JournalStore?.Save(Journal);
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }

    private static Task<byte[]> HandleRequest(Session session, NxExecutor executor, byte[] payload, CancellationToken token)
    {
        WireMessageProbeDto probe;
        try
        {
            probe = Wire.Deserialize<WireMessageProbeDto>(payload ?? Array.Empty<byte>());
        }
        catch (Exception ex)
        {
            return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid JSON request: " + ex.Message, false));
        }

        if (probe.ProtocolVersion != null && !string.IsNullOrWhiteSpace(probe.Nonce) && string.IsNullOrWhiteSpace(probe.RequestId))
        {
            try
            {
                var handshake = Wire.Deserialize<HandshakeRequestDto>(payload ?? Array.Empty<byte>());
                if (handshake.ProtocolVersion == null || string.IsNullOrWhiteSpace(handshake.Nonce))
                {
                    return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid handshake", false));
                }
                if (!string.IsNullOrWhiteSpace(ExpectedNonce) && !string.Equals(handshake.Nonce, ExpectedNonce, StringComparison.Ordinal))
                {
                    return Task.FromResult(FormatError(string.Empty, "UNAUTHORIZED", "handshake nonce mismatch: unauthorized client", false));
                }
                _authenticated = true;
                return Task.FromResult(FormatHandshake(session));
            }
            catch (Exception ex)
            {
                return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid handshake: " + ex.Message, false));
            }
        }

        if (!string.IsNullOrWhiteSpace(ExpectedNonce) && !_authenticated)
        {
            return Task.FromResult(FormatError(probe.RequestId ?? string.Empty, "UNAUTHORIZED", "worker authentication required: must handshake with valid nonce first", false));
        }

        RequestEnvelopeDto request;
        try
        {
            request = Wire.Deserialize<RequestEnvelopeDto>(payload ?? Array.Empty<byte>());
        }
        catch (Exception ex)
        {
            return Task.FromResult(FormatError(probe.RequestId ?? string.Empty, "INVALID_ARGUMENT", "invalid RPC envelope: " + ex.Message, false));
        }

        var requestId = request.RequestId ?? string.Empty;
        var operation = request.Operation ?? string.Empty;
        if (string.IsNullOrWhiteSpace(requestId) || string.IsNullOrWhiteSpace(operation))
        {
            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", "request_id and op are required", false));
        }
        if (!SupportedOperations.Contains(operation))
        {
            return Task.FromResult(FormatError(requestId, "UNSUPPORTED_OPERATION", "canonical NXHost operation is not supported: " + operation, false));
        }
        var requestPayload = request.Payload ?? new Dictionary<string, object>(StringComparer.Ordinal);

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
                admission = Journal.Admit(requestId, operation, Wire.Serialize(requestPayload), request.CorrelationId, request.TxId);
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
                        string syslogPath = string.Empty;
                        try { syslogPath = session.LogFile.FileName ?? string.Empty; } catch {}
                        return FormatResponse(requestId, new Dictionary<string, object>
                        {
                            ["release"] = release,
                            ["base_dir"] = baseDir,
                            ["thread_id"] = Environment.CurrentManagedThreadId,
                            ["epoch"] = Epoch,
                            ["session_id"] = SessionId,
                            ["syslog_path"] = syslogPath,
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
                case "part.get_attributes":
                    return StartPartGetAttributes(executor, requestId, requestPayload, token);
                case "part.set_attributes":
                    return StartPartSetAttributes(executor, requestId, requestPayload, token);
                case "part.bulk_metadata":
                    return StartPartBulkMetadata(session, executor, requestId, requestPayload, token);
                case "part.query_load_status":
                    return StartPartLoadStatus(executor, requestId, requestPayload, token);
                case "object.release":
                    return StartObjectRelease(executor, requestId, requestPayload, token);
                case "feature.create_block":
                    return StartCreateBlock(executor, requestId, requestPayload, token);
                case "feature.create_cylinder":
                    return StartCreateCylinder(executor, requestId, requestPayload, token);
                case "feature.boolean":
                    return StartBooleanOperation(executor, requestId, requestPayload, token);
                case "feature.create_hole":
                    return StartCreateHole(executor, requestId, requestPayload, token);
                case "part.query_bodies":
                    return StartQueryBodies(executor, requestId, requestPayload, token);
                case "geometry.query_mass_properties":
                    return StartMassProperties(executor, requestId, requestPayload, token);
                case "geometry.query_bounding_box":
                    return StartBoundingBox(executor, requestId, requestPayload, token);
                case "transaction.begin":
                    return StartTransactionBegin(session, executor, requestId, requestPayload, token);
                case "transaction.commit":
                    return StartTransactionCommit(session, executor, requestId, requestPayload, token);
                case "transaction.rollback":
                    return StartTransactionRollback(session, executor, requestId, requestPayload, token);
                case "assembly.add_component":
                    return StartAssemblyAddComponent(executor, requestId, requestPayload, token);
                case "assembly.query_tree":
                    return StartAssemblyQueryTree(executor, requestId, requestPayload, token);
                case "assembly.query_bom":
                    return StartAssemblyQueryBOM(executor, requestId, requestPayload, token);
                case "assembly.remove_component":
                    return StartAssemblyRemoveComponent(executor, requestId, requestPayload, token);
                case "drafting.create_sheet":
                    return StartDraftingCreateSheet(executor, requestId, requestPayload, token);
                case "drafting.query_sheets":
                    return StartDraftingQuerySheets(executor, requestId, requestPayload, token);
                case "drafting.export_pdf":
                    return StartDraftingExportPdf(executor, requestId, requestPayload, token);

                default:
                    return Task.FromResult(FormatError(requestId, "UNSUPPORTED_OPERATION", "canonical NXHost operation is not migrated yet: " + operation, false));
            }
        }
        catch (StaleObjectHandleException ex)
        {
            var response = FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);
            if (IsJournaledMutation(operation))
            {
                TryMarkFailedBeforeStart(requestId, ex.Message, response);
            }
            return Task.FromResult(response);
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
                    string.Equals(unitsText, "inch", StringComparison.OrdinalIgnoreCase) ||
                    string.Equals(unitsText, "inches", StringComparison.OrdinalIgnoreCase)
            ? Part.Units.Inches
            : Part.Units.Millimeters;

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            Journal.MarkStarted(requestId); PersistJournalOrThrow();
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
            Journal.MarkStarted(requestId); PersistJournalOrThrow();
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
        var saveAsPath = GetString(payload, "path", string.Empty);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(handle, "Part");
            Journal.MarkStarted(requestId); PersistJournalOrThrow();
            PartSaveStatus? saveStatus;
            if (!string.IsNullOrWhiteSpace(saveAsPath))
            {
                saveStatus = part.SaveAs(saveAsPath);
            }
            else
            {
                saveStatus = part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);
            }
            CheckAndDisposeSaveStatus(saveStatus);
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
        var save = GetBool(payload, "save", false);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(handle, "Part");
            Journal.MarkStarted(requestId); PersistJournalOrThrow();
            var name = part.Name;
            if (save)
            {
                var saveStatus = part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);
                CheckAndDisposeSaveStatus(saveStatus);
            }
            part.Close(BasePart.CloseWholeTree.False, BasePart.CloseModified.CloseModified, null!);
            Registry.ReleaseWithDependents(handle);
            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["closed"] = true,
                ["name"] = name,
            });
        }, token));
    }

    private static void CheckAndDisposeSaveStatus(PartSaveStatus? saveStatus)
    {
        if (saveStatus == null) return;
        try
        {
            if (saveStatus.NumberUnsavedParts > 0)
            {
                var errCode = saveStatus.GetStatus(0);
                if (errCode != 0)
                {
                    throw NXOpen.NXException.Create(errCode);
                }
                throw new InvalidOperationException("part save failed with unsaved parts count=" + saveStatus.NumberUnsavedParts);
            }
        }
        finally
        {
            saveStatus.Dispose();
        }
    }

    private static Task<byte[]> StartPartSummary(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(handle, "Part");
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

    private static Task<byte[]> StartPartGetAttributes(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");
        List<string>? titlesFilter = null;
        if (payload.TryGetValue("titles", out var titlesVal) && titlesVal is IEnumerable<object> titleList)
        {
            titlesFilter = titleList.Select(x => x?.ToString() ?? string.Empty).Where(x => !string.IsNullOrEmpty(x)).ToList();
        }

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(handle, "Part");
            var result = new List<Dictionary<string, object>>();

            if (titlesFilter != null && titlesFilter.Count > 0)
            {
                foreach (var title in titlesFilter)
                {
                    try
                    {
                        var strVal = part.GetStringAttribute(title);
                        result.Add(new Dictionary<string, object> { ["title"] = title, ["type"] = "string", ["value"] = strVal });
                    }
                    catch
                    {
                        try
                        {
                            var intVal = part.GetIntegerAttribute(title);
                            result.Add(new Dictionary<string, object> { ["title"] = title, ["type"] = "integer", ["value"] = intVal });
                        }
                        catch
                        {
                            try
                            {
                                var realVal = part.GetRealAttribute(title);
                                result.Add(new Dictionary<string, object> { ["title"] = title, ["type"] = "real", ["value"] = realVal });
                            }
                            catch { }
                        }
                    }
                }
            }
            else
            {
                try
                {
                    var userAttrs = part.GetUserAttributes();
                    if (userAttrs != null)
                    {
                        foreach (var attr in userAttrs)
                        {
                            object val = attr.StringValue ?? (object?)attr.IntegerValue ?? attr.RealValue ?? string.Empty;
                            string typeStr = attr.Type.ToString().ToLowerInvariant();
                            result.Add(new Dictionary<string, object>
                            {
                                ["title"] = attr.Title ?? string.Empty,
                                ["type"] = typeStr,
                                ["value"] = val
                            });
                        }
                    }
                }
                catch { }
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["attributes"] = result
            });
        }, token));
    }

    private static Task<byte[]> StartPartSetAttributes(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");
        if (!payload.TryGetValue("attributes", out var attrsVal) || !(attrsVal is IEnumerable<object> attrList))
        {
            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", "attributes list is required", false));
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(handle, "Part");
            Journal.MarkStarted(requestId); PersistJournalOrThrow();
            int updated = 0;

            foreach (var item in attrList)
            {
                if (item is Dictionary<string, object> dict && dict.TryGetValue("title", out var titleObj) && titleObj != null)
                {
                    var title = titleObj.ToString()!;
                    var type = dict.TryGetValue("type", out var typeObj) ? typeObj?.ToString()?.ToLowerInvariant() : "string";
                    dict.TryGetValue("value", out var valObj);

                    switch (type)
                    {
                        case "integer":
                            part.SetAttribute(title, Convert.ToInt32(valObj, CultureInfo.InvariantCulture));
                            break;
                        case "real":
                            part.SetAttribute(title, Convert.ToDouble(valObj, CultureInfo.InvariantCulture));
                            break;
                        default:
                            part.SetAttribute(title, valObj?.ToString() ?? string.Empty);
                            break;
                    }
                    updated++;
                }
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["updated_count"] = updated
            });
        }, token));
    }

    private static Task<byte[]> StartPartBulkMetadata(Session session, NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var refs = new List<ObjectHandleToken>();
        if (payload.TryGetValue("part_refs", out var pRefsObj) && pRefsObj is IEnumerable<object> pList)
        {
            foreach (var item in pList)
            {
                if (item is Dictionary<string, object> dict)
                {
                    refs.Add(RequireHandle(new Dictionary<string, object> { ["part"] = dict }, "part", "Part"));
                }
            }
        }
        var includeAttrs = payload.TryGetValue("include_attributes", out var incVal) && Convert.ToBoolean(incVal);

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var targetParts = new List<Part>();
            if (refs.Count > 0)
            {
                foreach (var h in refs)
                {
                    targetParts.Add((Part)Registry.Resolve(h, "Part"));
                }
            }
            else
            {
                foreach (Part p in session.Parts)
                {
                    targetParts.Add(p);
                }
            }

            var entries = new List<Dictionary<string, object>>();
            foreach (var part in targetParts)
            {
                var bodyCount = 0;
                foreach (Body _ in part.Bodies) bodyCount++;
                var featureCount = 0;
                foreach (NXOpen.Features.Feature _ in part.Features) featureCount++;
                var componentCount = 0;
                if (part.ComponentAssembly != null && part.ComponentAssembly.RootComponent != null)
                {
                    componentCount = part.ComponentAssembly.RootComponent.GetChildren().Length;
                }

                var partHandle = Registry.GetOrRegister(part, "Part");
                var entry = new Dictionary<string, object>
                {
                    ["part_ref"] = FormatHandle(partHandle, part),
                    ["name"] = part.Name,
                    ["full_path"] = part.FullPath ?? string.Empty,
                    ["units"] = part.PartUnits.ToString(),
                    ["is_modified"] = false,
                    ["body_count"] = bodyCount,
                    ["feature_count"] = featureCount,
                    ["component_count"] = componentCount,
                };

                if (includeAttrs)
                {
                    var attrList = new List<Dictionary<string, object>>();
                    try
                    {
                        var userAttrs = part.GetUserAttributes();
                        if (userAttrs != null)
                        {
                            foreach (var attr in userAttrs)
                            {
                                object val = attr.StringValue ?? (object?)attr.IntegerValue ?? attr.RealValue ?? string.Empty;
                                attrList.Add(new Dictionary<string, object>
                                {
                                    ["title"] = attr.Title ?? string.Empty,
                                    ["type"] = attr.Type.ToString().ToLowerInvariant(),
                                    ["value"] = val
                                });
                            }
                        }
                    }
                    catch { }
                    entry["attributes"] = attrList;
                }

                entries.Add(entry);
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["entries"] = entries
            });
        }, token));
    }

    private static Task<byte[]> StartPartLoadStatus(NxExecutor executor, string requestId, Dictionary<string, object> payload, CancellationToken token)
    {
        var handle = RequireHandle(payload, "part_ref", "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(handle, "Part");

            var isFullyLoaded = false;
            var isModified = false;
            var isReadOnly = false;
            var hasWriteAccess = true;
            var loadState = string.Empty;

            try { isFullyLoaded = part.IsFullyLoaded; } catch { }
            try { isModified = part.IsModified; } catch { }
            try { isReadOnly = part.IsReadOnly; } catch { }
            try { hasWriteAccess = part.HasWriteAccess; } catch { }
            try { loadState = part.PartLoadState.ToString(); } catch { }

            var unloadedDeps = new List<Dictionary<string, object>>();
            try
            {
                var pls = part.LoadFeatureDataForSelection();
                if (pls != null)
                {
                    for (int i = 0; i < pls.NumberUnloadedParts; i++)
                    {
                        unloadedDeps.Add(new Dictionary<string, object>
                        {
                            ["part_name"] = pls.GetPartName(i),
                            ["status_code"] = pls.GetStatus(i),
                            ["status_description"] = pls.GetStatusDescription(i) ?? string.Empty,
                        });
                    }
                }
            }
            catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["is_fully_loaded"] = isFullyLoaded,
                ["is_modified"] = isModified,
                ["is_read_only"] = isReadOnly,
                ["has_write_access"] = hasWriteAccess,
                ["load_state"] = loadState,
                ["unloaded_dependencies"] = unloadedDeps,
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
        catch (ArgumentException ex)
        {
            return FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);
        }
        catch (HandleRegistryCapacityException ex)
        {
            return FormatError(requestId, "CAPACITY", ex.Message, true);
        }
        catch (HandleScopeCapacityException ex)
        {
            return FormatError(requestId, "CAPACITY", ex.Message, true);
        }
        catch (StaleObjectHandleException ex)
        {
            return FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);
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
            Journal.MarkCommitted(requestId, response); PersistJournalOrThrow();
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
            if (Journal.TryGet(requestId, out record) && record != null)
            {
                if (record.State == RequestJournalState.Started)
                {
                    Journal.MarkOutcomeUnknown(requestId, ex.GetType().Name + ": " + ex.Message); PersistJournalOrThrow();
                    Health.MarkLost();
                    return FormatError(requestId, "OUTCOME_UNKNOWN", "NX mutation faulted after execution started; worker is quarantined: " + ex.Message, false);
                }
                if (record.State == RequestJournalState.Received)
                {
                    var category = PreStartErrorCategory(ex);
                    var response = FormatError(requestId, category, ex.GetType().Name + ": " + ex.Message, category == "CAPACITY");
                    TryMarkFailedBeforeStart(requestId, ex.Message, response);
                    return response;
                }
            }

            Health.MarkSuspect();
            return FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false);
        }
    }

    private static string PreStartErrorCategory(Exception ex)
    {
        if (ex is ArgumentException || ex is KeyNotFoundException || ex is StaleObjectHandleException)
        {
            return "INVALID_ARGUMENT";
        }
        if (ex is UndoTransactionCapacityException ||
            ex is HandleRegistryCapacityException ||
            ex is HandleScopeCapacityException)
        {
            return "CAPACITY";
        }
        return "INTERNAL";
    }

    private static void TryMarkFailedBeforeStart(string requestId, string diagnostic, byte[] response)
    {
        RequestJournalRecord? record;
        if (Journal.TryGet(requestId, out record) && record != null && record.State == RequestJournalState.Received)
        {
            Journal.MarkFailed(requestId, string.IsNullOrWhiteSpace(diagnostic) ? "request failed before NX execution" : diagnostic, response); PersistJournalOrThrow();
        }
    }

    private static bool IsJournaledMutation(string operation)
    {
        return MutationClasses.ContainsKey(operation);
    }

    private static byte[] FormatHandshake(Session session)
    {
        var release = session.GetEnvironmentVariableValue("UGII_VERSION");
        if (string.IsNullOrWhiteSpace(release)) release = "unknown";
        return Wire.Serialize(new HandshakeResponseDto
        {
            ProtocolVersion = new ProtocolVersionDto { Major = ProtocolMajor, Minor = ProtocolMinor },
            AgentVersion = "v0.2.0-nxhost",
            NxRelease = release,
            NxBuild = release + ".compiled",
            NxPid = Process.GetCurrentProcess().Id,
            SessionId = SessionId,
            Epoch = Epoch,
            Capabilities = new List<string>(SupportedOperations).ToArray(),
            MaxPayloadBytes = JsonWireCodec.DefaultMaxPayloadBytes,
            SecurityPolicy = "local_pipe_only",
        });
    }

    private static byte[] FormatResponse(string requestId, Dictionary<string, object> payload)
    {
        return Wire.Serialize(new ResponseEnvelopeDto
        {
            RequestId = requestId ?? string.Empty,
            Status = "OK",
            Payload = payload ?? new Dictionary<string, object>(StringComparer.Ordinal),
        });
    }

    private static byte[] FormatError(string requestId, string category, string message, bool recoverable)
    {
        return Wire.Serialize(new ResponseEnvelopeDto
        {
            RequestId = requestId ?? string.Empty,
            Status = "ERROR",
            Error = new WireErrorDto
            {
                Category = category ?? string.Empty,
                NxErrorCode = 0,
                Message = message ?? string.Empty,
                Recoverable = recoverable,
                SessionHealth = WireHealth(),
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

    private static Dictionary<string, object>? GetObject(Dictionary<string, object> source, string key, bool required)
    {
        object? value;
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
        object? value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        return Convert.ToString(value, CultureInfo.InvariantCulture) ?? defaultValue;
    }

    private static bool GetBool(Dictionary<string, object> source, string key, bool defaultValue)
    {
        object? value;
        if (!source.TryGetValue(key, out value) || value == null) return defaultValue;
        if (value is bool) return (bool)value;
        bool parsed;
        return bool.TryParse(Convert.ToString(value, CultureInfo.InvariantCulture), out parsed) ? parsed : defaultValue;
    }

    private static ulong GetUInt64(Dictionary<string, object> source, string key)
    {
        object? value;
        if (!source.TryGetValue(key, out value) || value == null) throw new ArgumentException("missing numeric field: " + key);
        return Convert.ToUInt64(value, CultureInfo.InvariantCulture);
    }

    private static uint GetUInt32(Dictionary<string, object> source, string key)
    {
        object? value;
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

    private static List<ObjectHandleToken> ExtractHandleList(Dictionary<string, object> payload, string key, string expectedKind)
    {
        var list = new List<ObjectHandleToken>();
        if (payload.TryGetValue(key, out var raw) && raw is IEnumerable<object> items)
        {
            foreach (var it in items)
            {
                if (it is Dictionary<string, object> dict)
                {
                    list.Add(RequireHandle(new Dictionary<string, object> { ["item"] = dict }, "item", expectedKind));
                }
            }
        }
        return list;
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
