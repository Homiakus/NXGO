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

    [DataMember(Name = "requested_mode", Order = 3, EmitDefaultValue = false)]
    public string? RequestedMode { get; set; }

    [DataMember(Name = "requested_features", Order = 4, EmitDefaultValue = false)]
    public string[]? RequestedFeatures { get; set; }

    [DataMember(Name = "client_pid", Order = 5)]
    public int ClientPid { get; set; }

    [DataMember(Name = "client_user", Order = 6, EmitDefaultValue = false)]
    public string? ClientUser { get; set; }

    [DataMember(Name = "nonce", Order = 7)]
    public string Nonce { get; set; } = string.Empty;
}

[DataContract]
public sealed class HandshakeResponseDto
{
    [DataMember(Name = "protocol_version", Order = 1)]
    public ProtocolVersionDto ProtocolVersion { get; set; } = new ProtocolVersionDto();

    [DataMember(Name = "agent_version", Order = 2)]
    public string AgentVersion { get; set; } = string.Empty;

    [DataMember(Name = "nx_release", Order = 3, EmitDefaultValue = false)]
    public string? NxRelease { get; set; }

    [DataMember(Name = "nx_build", Order = 4, EmitDefaultValue = false)]
    public string? NxBuild { get; set; }

    [DataMember(Name = "nx_pid", Order = 5)]
    public int NxPid { get; set; }

    [DataMember(Name = "session_id", Order = 6)]
    public string SessionId { get; set; } = string.Empty;

    [DataMember(Name = "epoch", Order = 7)]
    public ulong Epoch { get; set; }

    [DataMember(Name = "capabilities", Order = 8, EmitDefaultValue = false)]
    public string[]? Capabilities { get; set; }

    [DataMember(Name = "max_payload_bytes", Order = 9)]
    public int MaxPayloadBytes { get; set; }

    [DataMember(Name = "security_policy", Order = 10, EmitDefaultValue = false)]
    public string? SecurityPolicy { get; set; }
}

[DataContract]
public sealed class RequestEnvelopeDto
{
    [DataMember(Name = "request_id", Order = 1)]
    public string RequestId { get; set; } = string.Empty;

    [DataMember(Name = "correlation_id", Order = 2, EmitDefaultValue = false)]
    public string? CorrelationId { get; set; }

    [DataMember(Name = "run_id", Order = 3, EmitDefaultValue = false)]
    public string? RunId { get; set; }

    [DataMember(Name = "test_id", Order = 4, EmitDefaultValue = false)]
    public string? TestId { get; set; }

    [DataMember(Name = "op", Order = 5)]
    public string Operation { get; set; } = string.Empty;

    [DataMember(Name = "timeout_ms", Order = 6, EmitDefaultValue = false)]
    public long TimeoutMs { get; set; }

    [DataMember(Name = "tx_id", Order = 7, EmitDefaultValue = false)]
    public string? TxId { get; set; }

    [DataMember(Name = "payload", Order = 8, EmitDefaultValue = false)]
    public Dictionary<string, object>? Payload { get; set; }

    [DataMember(Name = "trace_meta", Order = 9, EmitDefaultValue = false)]
    public Dictionary<string, string>? TraceMeta { get; set; }
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

    [DataMember(Name = "warnings", Order = 5, EmitDefaultValue = false)]
    public string[]? Warnings { get; set; }

    [DataMember(Name = "produced_handles", Order = 6, EmitDefaultValue = false)]
    public ObjectHandleWireDto[]? ProducedHandles { get; set; }

    [DataMember(Name = "timing", Order = 7, EmitDefaultValue = false)]
    public TimingDataDto? Timing { get; set; }
}

[DataContract]
public sealed class WireErrorDto
{
    [DataMember(Name = "category", Order = 1)]
    public string Category { get; set; } = string.Empty;

    [DataMember(Name = "nx_error_code", Order = 2, EmitDefaultValue = false)]
    public int NxErrorCode { get; set; }

    [DataMember(Name = "message", Order = 3)]
    public string Message { get; set; } = string.Empty;

    [DataMember(Name = "op", Order = 4, EmitDefaultValue = false)]
    public string? Operation { get; set; }

    [DataMember(Name = "recoverable", Order = 5)]
    public bool Recoverable { get; set; }

    [DataMember(Name = "session_health", Order = 6)]
    public string SessionHealth { get; set; } = string.Empty;

    [DataMember(Name = "correlation_id", Order = 7, EmitDefaultValue = false)]
    public string? CorrelationId { get; set; }

    [DataMember(Name = "diagnostic", Order = 8, EmitDefaultValue = false)]
    public string? Diagnostic { get; set; }
}

[DataContract]
public sealed class TimingDataDto
{
    [DataMember(Name = "queue_wait_ms", Order = 1, EmitDefaultValue = false)]
    public long QueueWaitMs { get; set; }

    [DataMember(Name = "execution_ms", Order = 2, EmitDefaultValue = false)]
    public long ExecutionMs { get; set; }

    [DataMember(Name = "serialize_ms", Order = 3, EmitDefaultValue = false)]
    public long SerializeMs { get; set; }

    [DataMember(Name = "total_duration_ms", Order = 4, EmitDefaultValue = false)]
    public long TotalDurationMs { get; set; }
}

[DataContract]
public sealed class ObjectHandleWireDto
{
    [DataMember(Name = "session_id", Order = 1)]
    public string SessionId { get; set; } = string.Empty;

    [DataMember(Name = "epoch", Order = 2)]
    public ulong Epoch { get; set; }

    [DataMember(Name = "object_id", Order = 3)]
    public string ObjectId { get; set; } = string.Empty;

    [DataMember(Name = "generation", Order = 4)]
    public uint Generation { get; set; }

    [DataMember(Name = "kind", Order = 5)]
    public string Kind { get; set; } = string.Empty;

    [DataMember(Name = "native_tag", Order = 6, EmitDefaultValue = false)]
    public uint NativeTag { get; set; }

    [DataMember(Name = "lease_scope_id", Order = 7, EmitDefaultValue = false)]
    public string? LeaseScopeId { get; set; }
}

[DataContract]
public sealed class ObjectReleaseRequestDto
{
    [DataMember(Name = "handles", Order = 1, EmitDefaultValue = false)]
    public ObjectHandleWireDto[]? Handles { get; set; }

    [DataMember(Name = "lease_scope_id", Order = 2, EmitDefaultValue = false)]
    public string? LeaseScopeId { get; set; }
}
