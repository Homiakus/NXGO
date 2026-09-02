using System.Diagnostics;
using System.Globalization;
using System.Text;
using NXGO.Agent.Core;
using NXOpen;

namespace NXGO.Agent.NXHost;

/// <summary>
/// Dedicated-worker bootstrap for the canonical compiled Agent path.
/// Transport threads only parse/enqueue. All NXOpen work executes through the
/// shared Agent.Core NxExecutor on the bound NX execution thread.
/// </summary>
public static class EntryPoint
{
    private const int ProtocolMajor = 2;
    private const int ProtocolMinor = 0;
    private static readonly SessionHealthState Health = new SessionHealthState();
    private static readonly string SessionId = "nxgo-" + Guid.NewGuid().ToString("N");
    private const ulong Epoch = 1;
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

            session.LogFile.WriteLine($"[NXGO] canonical NXHost stop health={Health.Value}");
        }
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }

    private static Task<byte[]> HandleRequest(Session session, NxExecutor executor, byte[] payload, CancellationToken token)
    {
        var request = Encoding.UTF8.GetString(payload);

        if (request.Contains("\"protocol_version\"") && request.Contains("\"nonce\""))
        {
            var release = session.GetEnvironmentVariableValue("UGII_VERSION");
            if (string.IsNullOrWhiteSpace(release)) release = "unknown";
            var handshake = string.Format(
                CultureInfo.InvariantCulture,
                "{{\"protocol_version\":{{\"major\":{0},\"minor\":{1}}},\"agent_version\":\"v0.2.0-nxhost\",\"nx_release\":\"{2}\",\"nx_build\":\"{2}.compiled\",\"nx_pid\":{3},\"session_id\":\"{4}\",\"epoch\":{5},\"capabilities\":[\"nx.ping\",\"session.info\",\"shutdown\"],\"max_payload_bytes\":4194304,\"security_policy\":\"local_pipe_only\"}}",
                ProtocolMajor,
                ProtocolMinor,
                EscapeJson(release),
                Process.GetCurrentProcess().Id,
                SessionId,
                Epoch);
            return Task.FromResult(Encoding.UTF8.GetBytes(handshake));
        }

        var requestId = ExtractJsonString(request, "request_id");
        var operation = ExtractJsonString(request, "op");
        if (string.IsNullOrWhiteSpace(requestId) || string.IsNullOrWhiteSpace(operation))
        {
            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", "request_id and op are required"));
        }

        switch (operation)
        {
            case "nx.ping":
                return Map(requestId, executor.Enqueue(() =>
                {
                    Health.RequireReusable();
                    session.LogFile.WriteLine("[NXGO] canonical nx.ping");
                    return FormatResponse(requestId, "{\"ping\":\"pong\"}");
                }, token));

            case "session.info":
                return Map(requestId, executor.Enqueue(() =>
                {
                    Health.RequireReusable();
                    var release = session.GetEnvironmentVariableValue("UGII_VERSION");
                    var baseDir = session.GetEnvironmentVariableValue("UGII_BASE_DIR");
                    var info = string.Format(
                        CultureInfo.InvariantCulture,
                        "{{\"release\":\"{0}\",\"base_dir\":\"{1}\",\"thread_id\":{2},\"epoch\":{3},\"session_id\":\"{4}\"}}",
                        EscapeJson(release ?? string.Empty),
                        EscapeJson(baseDir ?? string.Empty),
                        Environment.CurrentManagedThreadId,
                        Epoch,
                        SessionId);
                    return FormatResponse(requestId, info);
                }, token));

            case "shutdown":
                _shutdownRequested = true;
                return Task.FromResult(FormatResponse(requestId, "{\"shutdown\":true}"));

            default:
                return Task.FromResult(FormatError(requestId, "UNSUPPORTED_OPERATION", "canonical NXHost operation is not migrated yet: " + operation));
        }
    }

    private static async Task<byte[]> Map(string requestId, Task<byte[]> task)
    {
        try
        {
            return await task.ConfigureAwait(false);
        }
        catch (TaskCanceledException ex)
        {
            return FormatError(requestId, "CANCELLED_BEFORE_START", ex.Message);
        }
        catch (Exception ex)
        {
            Health.MarkSuspect();
            return FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message);
        }
    }

    private static byte[] FormatResponse(string requestId, string payloadJson)
    {
        var json = string.Format(
            CultureInfo.InvariantCulture,
            "{{\"request_id\":\"{0}\",\"status\":\"OK\",\"payload\":{1}}}",
            EscapeJson(requestId),
            string.IsNullOrWhiteSpace(payloadJson) ? "{}" : payloadJson);
        return Encoding.UTF8.GetBytes(json);
    }

    private static byte[] FormatError(string requestId, string category, string message)
    {
        var json = string.Format(
            CultureInfo.InvariantCulture,
            "{{\"request_id\":\"{0}\",\"status\":\"ERROR\",\"error\":{{\"category\":\"{1}\",\"nx_error_code\":0,\"message\":\"{2}\",\"recoverable\":false,\"session_health\":\"{3}\"}}}}",
            EscapeJson(requestId ?? string.Empty),
            EscapeJson(category),
            EscapeJson(message),
            Health.Value.ToString().ToLowerInvariant());
        return Encoding.UTF8.GetBytes(json);
    }

    private static string ExtractJsonString(string json, string key)
    {
        if (string.IsNullOrEmpty(json)) return string.Empty;
        var token = "\"" + key + "\"";
        var keyIndex = json.IndexOf(token, StringComparison.Ordinal);
        if (keyIndex < 0) return string.Empty;
        var colon = json.IndexOf(':', keyIndex + token.Length);
        if (colon < 0) return string.Empty;
        var firstQuote = json.IndexOf('"', colon + 1);
        if (firstQuote < 0) return string.Empty;

        var sb = new StringBuilder();
        var escaped = false;
        for (var i = firstQuote + 1; i < json.Length; i++)
        {
            var ch = json[i];
            if (escaped)
            {
                switch (ch)
                {
                    case 'n': sb.Append('\n'); break;
                    case 'r': sb.Append('\r'); break;
                    case 't': sb.Append('\t'); break;
                    default: sb.Append(ch); break;
                }
                escaped = false;
                continue;
            }
            if (ch == '\\')
            {
                escaped = true;
                continue;
            }
            if (ch == '"') return sb.ToString();
            sb.Append(ch);
        }
        return string.Empty;
    }

    private static string EscapeJson(string value)
    {
        if (string.IsNullOrEmpty(value)) return string.Empty;
        return value
            .Replace("\\", "\\\\")
            .Replace("\"", "\\\"")
            .Replace("\r", "\\r")
            .Replace("\n", "\\n")
            .Replace("\t", "\\t");
    }
}
