#!/usr/bin/env python3
from pathlib import Path

host_path = Path('agent/NXGO.Agent.NXHost/EntryPoint.cs')
text = host_path.read_text(encoding='utf-8')


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
new_helpers = '''    private static byte[] FormatHandshake(Session session)\n    {\n        var release = session.GetEnvironmentVariableValue("UGII_VERSION");\n        if (string.IsNullOrWhiteSpace(release)) release = "unknown";\n        return Wire.Serialize(new HandshakeResponseDto\n        {\n            ProtocolVersion = new ProtocolVersionDto { Major = ProtocolMajor, Minor = ProtocolMinor },\n            AgentVersion = "v0.2.0-nxhost",\n            NxRelease = release,\n            NxBuild = release + ".compiled",\n            NxPid = Process.GetCurrentProcess().Id,\n            SessionId = SessionId,\n            Epoch = Epoch,\n            Capabilities = new[]\n            {\n                "nx.ping",\n                "session.info",\n                "part.new",\n                "part.open",\n                "part.save",\n                "part.close",\n                "part.query_summary",\n                "object.release",\n                "feature.create_block",\n                "feature.create_cylinder",\n                "part.query_bodies",\n                "geometry.query_mass_properties",\n                "geometry.query_bounding_box",\n                "transaction.begin",\n                "transaction.commit",\n                "transaction.rollback",\n                "assembly.add_component",\n                "assembly.query_tree",\n                "assembly.query_bom",\n                "assembly.remove_component",\n                "drafting.create_sheet",\n                "drafting.query_sheets",\n                "drafting.export_pdf",\n                "shutdown",\n            },\n            MaxPayloadBytes = JsonWireCodec.DefaultMaxPayloadBytes,\n            SecurityPolicy = "local_pipe_only",\n        });\n    }\n\n    private static byte[] FormatResponse(string requestId, Dictionary<string, object> payload)\n    {\n        return Wire.Serialize(new ResponseEnvelopeDto\n        {\n            RequestId = requestId ?? string.Empty,\n            Status = "OK",\n            Payload = payload ?? new Dictionary<string, object>(StringComparer.Ordinal),\n        });\n    }\n\n    private static byte[] FormatError(string requestId, string category, string message, bool recoverable)\n    {\n        return Wire.Serialize(new ResponseEnvelopeDto\n        {\n            RequestId = requestId ?? string.Empty,\n            Status = "ERROR",\n            Error = new WireErrorDto\n            {\n                Category = category ?? string.Empty,\n                NxErrorCode = 0,\n                Message = message ?? string.Empty,\n                Recoverable = recoverable,\n                SessionHealth = WireHealth(),\n            },\n        });\n    }\n\n    private static string WireHealth()\n    {\n        return Health.Value == SessionHealth.Healthy\n            ? "healthy"\n            : Health.Value == SessionHealth.Lost\n                ? "lost"\n                : "dirty";\n    }\n\n'''
text = text[:start] + new_helpers + text[end:]

for forbidden in ('JavaScriptSerializer', 'System.Web.Script.Serialization', 'DecodeObject(', 'Json.Serialize(', 'Json.DeserializeObject'):
    if forbidden in text:
        raise SystemExit(f'forbidden serializer marker remains: {forbidden}')
for required in ('JsonWireCodec', 'WireMessageProbeDto', 'RequestEnvelopeDto', 'HandshakeResponseDto', 'ResponseEnvelopeDto', 'WireErrorDto'):
    if required not in text:
        raise SystemExit(f'required protocol marker missing: {required}')
host_path.write_text(text, encoding='utf-8')

project_path = Path('agent/NXGO.Agent.NXHost/NXGO.Agent.NXHost.csproj')
project = project_path.read_text(encoding='utf-8')
old_ref = '    <Reference Include="System.Web.Extensions" />\n'
if project.count(old_ref) != 1:
    raise SystemExit(f'System.Web.Extensions reference count={project.count(old_ref)}')
project = project.replace(old_ref, '', 1)
if 'NXGO.Protocol\\NXGO.Protocol.csproj' not in project:
    raise SystemExit('NXHost project is missing NXGO.Protocol ProjectReference')
project_path.write_text(project, encoding='utf-8')

guard_path = Path('internal/agentbundle/compiled_host_test.go')
guard = guard_path.read_text(encoding='utf-8')
old = '''\tif !strings.Contains(project, "<ProjectReference") || !strings.Contains(project, "NXGO.Agent.Core") {\n\t\tt.Fatal("NXHost must consume NXGO.Agent.Core through a ProjectReference")\n\t}\n\tif !strings.Contains(project, "System.Web.Extensions") {\n\t\tt.Fatal("canonical NXHost must use the framework JSON serializer rather than manual JSON slicing")\n\t}\n'''
new = '''\tif !strings.Contains(project, "<ProjectReference") || !strings.Contains(project, "NXGO.Agent.Core") || !strings.Contains(project, "NXGO.Protocol") {\n\t\tt.Fatal("NXHost must consume NXGO.Agent.Core and NXGO.Protocol through ProjectReferences")\n\t}\n\tif strings.Contains(project, "System.Web.Extensions") {\n\t\tt.Fatal("canonical NXHost must not retain the legacy JavaScriptSerializer dependency")\n\t}\n'''
if guard.count(old) != 1:
    raise SystemExit('compiled-host project guard source mismatch')
guard = guard.replace(old, new, 1)
guard = guard.replace('\t\t"JavaScriptSerializer",\n', '\t\t"JsonWireCodec",\n\t\t"WireMessageProbeDto",\n\t\t"RequestEnvelopeDto",\n\t\t"ResponseEnvelopeDto",\n', 1)
guard = guard.replace('\t\t"NXGO.Agent.Core.dll",\n\t\t"NXGO.Agent.NXHost.dll",\n', '\t\t"Newtonsoft.Json.dll",\n\t\t"NXGO.Protocol.dll",\n\t\t"NXGO.Agent.Core.dll",\n\t\t"NXGO.Agent.NXHost.dll",\n', 1)
old_build = '''\tif !strings.Contains(build, "dotnet build $hostProject") ||\n\t\t!strings.Contains(build, "NXGO.Agent.Core.dll") ||\n\t\t!strings.Contains(build, "NXGO.Agent.NXHost.dll") {\n\t\tt.Fatal("build-agent.ps1 must build and verify canonical Core/NXHost outputs")\n\t}\n'''
new_build = '''\tif !strings.Contains(build, "dotnet build $hostProject") ||\n\t\t!strings.Contains(build, "NXGO.Protocol.dll") ||\n\t\t!strings.Contains(build, "Newtonsoft.Json.dll") ||\n\t\t!strings.Contains(build, "NXGO.Agent.Core.dll") ||\n\t\t!strings.Contains(build, "NXGO.Agent.NXHost.dll") {\n\t\tt.Fatal("build-agent.ps1 must build and verify canonical Protocol/Core/NXHost runtime outputs")\n\t}\n'''
if guard.count(old_build) != 1:
    raise SystemExit('compiled-host build guard source mismatch')
guard = guard.replace(old_build, new_build, 1)
for forbidden in ('JavaScriptSerializer', 'System.Web.Extensions'):
    # The forbidden strings are allowed only in the explicit negative guard text.
    pass
if 'JsonWireCodec' not in guard or 'NXGO.Protocol.dll' not in guard or 'Newtonsoft.Json.dll' not in guard:
    raise SystemExit('compiled-host guard did not acquire protocol runtime markers')
guard_path.write_text(guard, encoding='utf-8')

print('canonical NXHost typed protocol codec migration applied across host/project/guard')
