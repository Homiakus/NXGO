using NXGO.Protocol.V1;

namespace NXGO.Agent.Core;

public sealed class AgentIdentity
{
    public string AgentVersion { get; init; } = string.Empty;
    public string NxRelease { get; init; } = string.Empty;
    public string NxBuild { get; init; } = string.Empty;
    public ulong NxProcessId { get; init; }
    public string SessionId { get; init; } = string.Empty;
    public ulong SessionEpoch { get; init; }
    public SessionMode Mode { get; init; }
    public SessionHealth Health { get; init; }
    public IReadOnlyCollection<string> Capabilities { get; init; } = Array.Empty<string>();
    public uint MaxPayloadBytes { get; init; } = FrameCodec.DefaultMaxPayloadBytes;
    public uint MaxBatchItems { get; init; } = 128;
    public uint MaxInFlightRequests { get; init; } = 64;
}

public sealed class ProtocolNegotiationException : Exception
{
    public ProtocolNegotiationException(string message) : base(message) { }
}

public static class ProtocolNegotiator
{
    public const uint CurrentMajor = 1;
    public const uint CurrentMinor = 0;
    public const int MinimumNonceBytes = 16;
    public const int MaximumNonceBytes = 64;

    public static ServerHello Negotiate(ClientHello hello, AgentIdentity identity)
    {
        if (hello is null) throw new ArgumentNullException(nameof(hello));
        if (identity is null) throw new ArgumentNullException(nameof(identity));
        if (hello.Protocol is null)
        {
            throw new ProtocolNegotiationException("client protocol version is required");
        }
        if (hello.Protocol.Major != CurrentMajor)
        {
            throw new ProtocolNegotiationException($"protocol major mismatch: client={hello.Protocol.Major}, server={CurrentMajor}");
        }
        if (hello.ConnectionNonce.Length < MinimumNonceBytes || hello.ConnectionNonce.Length > MaximumNonceBytes)
        {
            throw new ProtocolNegotiationException($"connection nonce must be {MinimumNonceBytes}..{MaximumNonceBytes} bytes");
        }
        if ((int)hello.RequestedMode != 0 && hello.RequestedMode != identity.Mode)
        {
            throw new ProtocolNegotiationException($"requested session mode {(int)hello.RequestedMode} is unavailable");
        }

        var available = new HashSet<string>(identity.Capabilities, StringComparer.Ordinal);
        IEnumerable<string> negotiated;
        if (hello.RequestedCapabilities.Count == 0)
        {
            negotiated = available;
        }
        else
        {
            negotiated = hello.RequestedCapabilities.Where(available.Contains);
        }

        var response = new ServerHello
        {
            Protocol = new ProtocolVersion
            {
                Major = CurrentMajor,
                Minor = Math.Min(CurrentMinor, hello.Protocol.Minor)
            },
            AgentVersion = identity.AgentVersion,
            NxRelease = identity.NxRelease,
            NxBuild = identity.NxBuild,
            NxProcessId = identity.NxProcessId,
            SessionId = identity.SessionId,
            SessionEpoch = identity.SessionEpoch,
            Mode = identity.Mode,
            Health = identity.Health,
            Limits = new ServerLimits
            {
                MaxPayloadBytes = identity.MaxPayloadBytes,
                MaxBatchItems = identity.MaxBatchItems,
                MaxInFlightRequests = identity.MaxInFlightRequests
            }
        };
        response.Capabilities.Add(negotiated.Distinct(StringComparer.Ordinal).OrderBy(x => x, StringComparer.Ordinal));
        return response;
    }
}
