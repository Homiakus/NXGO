#!/usr/bin/env python3
from pathlib import Path

path = Path('agent/NXGO.Agent.NXHost/EntryPoint.cs')
text = path.read_text(encoding='utf-8')


def replace_once(old: str, new: str, label: str) -> None:
    global text
    count = text.count(old)
    if count != 1:
        raise SystemExit(f'{label}: expected exactly one source snippet, got {count}')
    text = text.replace(old, new, 1)


replace_once(
    'using System.Text;\nusing System.Threading;\nusing System.Threading.Tasks;\nusing System.Web.Script.Serialization;\nusing NXGO.Agent.Core;\n',
    'using System.Threading;\nusing System.Threading.Tasks;\nusing NXGO.Agent.Core;\nusing NXGO.Protocol;\n',
    'serializer usings',
)

replace_once(
    '    private static readonly JavaScriptSerializer Json = new JavaScriptSerializer { MaxJsonLength = 4 * 1024 * 1024 };\n',
    '    private static readonly JsonWireCodec Wire = new JsonWireCodec(JsonWireCodec.DefaultMaxPayloadBytes);\n',
    'serializer field',
)

old_decode = '''        Dictionary<string, object> envelope;\n        try\n        {\n            envelope = DecodeObject(payload);\n        }\n        catch (Exception ex)\n        {\n            return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid JSON request: " + ex.Message, false));\n        }\n\n        if (envelope.ContainsKey("protocol_version") && envelope.ContainsKey("nonce") && !envelope.ContainsKey("request_id"))\n        {\n            return Task.FromResult(FormatHandshake(session));\n        }\n\n        var requestId = GetString(envelope, "request_id", string.Empty);\n        var operation = GetString(envelope, "op", string.Empty);\n        if (string.IsNullOrWhiteSpace(requestId) || string.IsNullOrWhiteSpace(operation))\n        {\n            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", "request_id and op are required", false));\n        }\n        var requestPayload = GetObject(envelope, "payload", required: false) ?? new Dictionary<string, object>(StringComparer.Ordinal);\n'''
new_decode = '''        WireMessageProbeDto probe;\n        try\n        {\n            probe = Wire.Deserialize<WireMessageProbeDto>(payload ?? Array.Empty<byte>());\n        }\n        catch (Exception ex)\n        {\n            return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid JSON request: " + ex.Message, false));\n        }\n\n        if (probe.ProtocolVersion != null && !string.IsNullOrWhiteSpace(probe.Nonce) && string.IsNullOrWhiteSpace(probe.RequestId))\n        {\n            try\n            {\n                var handshake = Wire.Deserialize<HandshakeRequestDto>(payload);\n                if (handshake.ProtocolVersion == null || string.IsNullOrWhiteSpace(handshake.Nonce))\n                {\n                    return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid handshake", false));\n                }\n                return Task.FromResult(FormatHandshake(session));\n            }\n            catch (Exception ex)\n            {\n                return Task.FromResult(FormatError(string.Empty, "INVALID_ARGUMENT", "invalid handshake: " + ex.Message, false));\n            }\n        }\n\n        RequestEnvelopeDto request;\n        try\n        {\n            request = Wire.Deserialize<RequestEnvelopeDto>(payload);\n        }\n        catch (Exception ex)\n        {\n            return Task.FromResult(FormatError(probe.RequestId ?? string.Empty, "INVALID_ARGUMENT", "invalid RPC envelope: " + ex.Message, false));\n        }\n\n        var requestId = request.RequestId ?? string.Empty;\n        var operation = request.Operation ?? string.Empty;\n        if (string.IsNullOrWhiteSpace(requestId) || string.IsNullOrWhiteSpace(operation))\n        {\n            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", "request_id and op are required", false));\n        }\n        var requestPayload = request.Payload ?? new Dictionary<string, object>(StringComparer.Ordinal);\n'''
replace_once(old_decode, new_decode, 'typed envelope decode')

replace_once(
    '                admission = Journal.Admit(requestId, operation, Encoding.UTF8.GetBytes(Json.Serialize(requestPayload)));\n',
    '                admission = Journal.Admit(requestId, operation, Wire.Serialize(requestPayload));\n',
    'journal canonical payload serialization',
)

start = text.index('    private static byte[] FormatHandshake(Session session)\n')
end = text.index('    private static Dictionary<string, object>? GetObject(', start)
old_helpers = text[start:end]
new_helpers = '''    private static byte[] FormatHandshake(Session session)\n    {\n        var release = session.GetEnvironmentVariableValue("UGII_VERSION");\n        if (string.IsNullOrWhiteSpace(release)) release = "unknown";\n        return Wire.Serialize(new HandshakeResponseDto\n        {\n            ProtocolVersion = new ProtocolVersionDto { Major = ProtocolMajor, Minor = ProtocolMinor },\n            AgentVersion = "v0.2.0-nxhost",\n            NxRelease = release,\n            NxBuild = release + ".compiled",\n            NxPid = Process.GetCurrentProcess().Id,\n            SessionId = SessionId,\n            Epoch = Epoch,\n            Capabilities = new[]\n            {\n                "nx.ping",\n                "session.info",\n                "part.new",\n                "part.open",\n                "part.save",\n                "part.close",\n                "part.query_summary",\n                "object.release",\n                "feature.create_block",\n                "feature.create_cylinder",\n                "part.query_bodies",\n                "geometry.query_mass_properties",\n                "geometry.query_bounding_box",\n                "transaction.begin",\n                "transaction.commit",\n                "transaction.rollback",\n                "assembly.add_component",\n                "assembly.query_tree",\n                "assembly.query_bom",\n                "assembly.remove_component",\n                "drafting.create_sheet",\n                "drafting.query_sheets",\n                "drafting.export_pdf",\n                "shutdown",\n            },\n            MaxPayloadBytes = JsonWireCodec.DefaultMaxPayloadBytes,\n            SecurityPolicy = "local_pipe_only",\n        });\n    }\n\n    private static byte[] FormatResponse(string requestId, Dictionary<string, object> payload)\n    {\n        return Wire.Serialize(new ResponseEnvelopeDto\n        {\n            RequestId = requestId ?? string.Empty,\n            Status = "OK",\n            Payload = payload ?? new Dictionary<string, object>(StringComparer.Ordinal),\n        });\n    }\n\n    private static byte[] FormatError(string requestId, string category, string message, bool recoverable)\n    {\n        return Wire.Serialize(new ResponseEnvelopeDto\n        {\n            RequestId = requestId ?? string.Empty,\n            Status = "ERROR",\n            Error = new WireErrorDto\n            {\n                Category = category ?? string.Empty,\n                NxErrorCode = 0,\n                Message = message ?? string.Empty,\n                Recoverable = recoverable,\n                SessionHealth = WireHealth(),\n            },\n        });\n    }\n\n    private static string WireHealth()\n    {\n        return Health.Value == SessionHealth.Healthy\n            ? "healthy"\n            : Health.Value == SessionHealth.Lost\n                ? "lost"\n                : "dirty";\n    }\n\n'''
text = text[:start] + new_helpers + text[end:]

for forbidden in ('JavaScriptSerializer', 'System.Web.Script.Serialization', 'DecodeObject(', 'Json.Serialize(', 'Json.DeserializeObject'):
    if forbidden in text:
        raise SystemExit(f'forbidden serializer marker remains: {forbidden}')
for required in ('JsonWireCodec', 'WireMessageProbeDto', 'RequestEnvelopeDto', 'HandshakeResponseDto', 'ResponseEnvelopeDto', 'WireErrorDto'):
    if required not in text:
        raise SystemExit(f'required protocol marker missing: {required}')

path.write_text(text, encoding='utf-8')
print('canonical NXHost protocol codec migration applied')
