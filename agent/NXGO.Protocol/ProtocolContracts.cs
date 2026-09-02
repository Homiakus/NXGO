using System.Runtime.Serialization;

namespace NXGO.Protocol;

[DataContract]
public sealed class ProtocolVersionDto
{
    [DataMember(Name = "major", Order = 1)]
    public int Major { get; set; }

    [DataMember(Name = "minor", Order = 2)]
    public int Minor { get; set; }
}

/// <summary>
/// Minimal first-pass envelope used only to distinguish a handshake from a
/// normal RPC without exposing Json.NET DOM types to NXHost. Full validation is
/// performed by deserializing the selected typed DTO immediately afterward.
/// </summary>
[DataContract]
public sealed class WireMessageProbeDto
{
    [DataMember(Name = "protocol_version", Order = 1, EmitDefaultValue = false)]
    public ProtocolVersionDto? ProtocolVersion { get; set; }

    [DataMember(Name = "nonce", Order = 2, EmitDefaultValue = false)]
    public string? Nonce { get; set; }

    [DataMember(Name = "request_id", Order = 3, EmitDefaultValue = false)]
    public string? RequestId { get; set; }

    [DataMember(Name = "op", Order = 4, EmitDefaultValue = false)]
    public string? Operation { get; set; }
}

[DataContract]
public sealed class HandshakeRequestDto
{
    [DataMember(Name = "protocol_version", Order = 1)]
    public ProtocolVersionDto ProtocolVersion { get; set; } = new ProtocolVersionDto();

    [DataMember(Name = "sdk_version", Order = 2)]
    public string SdkVersion { get; set; } = string.Empty;

    [DataMember(Name = "client_pid", Order = 3)]
    public int ClientPid { get; set; }

    [DataMember(Name = "nonce", Order = 4)]
    public string Nonce { get; set; } = string.Empty;
}

[DataContract]
public sealed class HandshakeResponseDto
{
    [DataMember(Name = "protocol_version", Order = 1)]
    public ProtocolVersionDto ProtocolVersion { get; set; } = new ProtocolVersionDto();

    [DataMember(Name = "agent_version", Order = 2)]
    public string AgentVersion { get; set; } = string.Empty;

    [DataMember(Name = "nx_release", Order = 3)]
    public string NxRelease { get; set; } = string.Empty;

    [DataMember(Name = "nx_build", Order = 4)]
    public string NxBuild { get; set; } = string.Empty;

    [DataMember(Name = "nx_pid", Order = 5)]
    public int NxPid { get; set; }

    [DataMember(Name = "session_id", Order = 6)]
    public string SessionId { get; set; } = string.Empty;

    [DataMember(Name = "epoch", Order = 7)]
    public ulong Epoch { get; set; }

    [DataMember(Name = "capabilities", Order = 8)]
    public string[] Capabilities { get; set; } = Array.Empty<string>();

    [DataMember(Name = "max_payload_bytes", Order = 9)]
    public int MaxPayloadBytes { get; set; }

    [DataMember(Name = "security_policy", Order = 10)]
    public string SecurityPolicy { get; set; } = string.Empty;
}

[DataContract]
public sealed class RequestEnvelopeDto
{
    [DataMember(Name = "request_id", Order = 1)]
    public string RequestId { get; set; } = string.Empty;

    [DataMember(Name = "op", Order = 2)]
    public string Operation { get; set; } = string.Empty;

    [DataMember(Name = "payload", Order = 3)]
    public Dictionary<string, object> Payload { get; set; } = new Dictionary<string, object>(StringComparer.Ordinal);
}

[DataContract]
public sealed class ResponseEnvelopeDto
{
    [DataMember(Name = "request_id", Order = 1)]
    public string RequestId { get; set; } = string.Empty;

    [DataMember(Name = "status", Order = 2)]
    public string Status { get; set; } = string.Empty;

    [DataMember(Name = "payload", Order = 3, EmitDefaultValue = false)]
    public Dictionary<string, object>? Payload { get; set; }

    [DataMember(Name = "error", Order = 4, EmitDefaultValue = false)]
    public WireErrorDto? Error { get; set; }
}

[DataContract]
public sealed class WireErrorDto
{
    [DataMember(Name = "category", Order = 1)]
    public string Category { get; set; } = string.Empty;

    [DataMember(Name = "nx_error_code", Order = 2)]
    public int NxErrorCode { get; set; }

    [DataMember(Name = "message", Order = 3)]
    public string Message { get; set; } = string.Empty;

    [DataMember(Name = "recoverable", Order = 4)]
    public bool Recoverable { get; set; }

    [DataMember(Name = "session_health", Order = 5)]
    public string SessionHealth { get; set; } = string.Empty;
}
