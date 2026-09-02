using System.Runtime.Serialization;
using System.Text;
using System.Text.Json;
using NXGO.Protocol;
using Xunit;

namespace NXGO.Protocol.Tests;

public sealed class JsonWireCodecTests
{
    [Fact]
    public void Handshake_roundtrip_preserves_snake_case_contract()
    {
        var codec = new JsonWireCodec();
        var input = new HandshakeResponseDto
        {
            ProtocolVersion = new ProtocolVersionDto { Major = 2, Minor = 0 },
            AgentVersion = "v0.2.0-nxhost",
            NxRelease = "2512.5000",
            NxBuild = "2512.5000.compiled",
            NxPid = 1234,
            SessionId = "nxgo-session-a",
            Epoch = 7,
            Capabilities = new[] { "nx.ping", "part.new" },
            MaxPayloadBytes = JsonWireCodec.DefaultMaxPayloadBytes,
            SecurityPolicy = "local_pipe_only",
        };

        var json = codec.SerializeUtf8(input);
        using var doc = JsonDocument.Parse(json);
        var root = doc.RootElement;
        Assert.Equal(2, root.GetProperty("protocol_version").GetProperty("major").GetInt32());
        Assert.Equal("nxgo-session-a", root.GetProperty("session_id").GetString());
        Assert.Equal("local_pipe_only", root.GetProperty("security_policy").GetString());
        Assert.False(root.TryGetProperty("ProtocolVersion", out _));

        var decoded = codec.Deserialize<HandshakeResponseDto>(Encoding.UTF8.GetBytes(json));
        Assert.Equal(input.ProtocolVersion.Major, decoded.ProtocolVersion.Major);
        Assert.Equal(input.SessionId, decoded.SessionId);
        Assert.NotNull(decoded.Capabilities);
        Assert.Equal(input.Capabilities, decoded.Capabilities!);
    }

    [Fact]
    public void Request_payload_roundtrip_handles_unicode_quotes_paths_controls_and_nested_values()
    {
        var codec = new JsonWireCodec();
        var request = new RequestEnvelopeDto
        {
            RequestId = "req-русский-日本語-\"quoted\"",
            Operation = "part.open",
            Payload = new Dictionary<string, object>(StringComparer.Ordinal)
            {
                ["path"] = "C:\\NXGO\\тест\\部品 {draft} \\\"quoted\\\".prt",
                ["note"] = "line1\nline2\twith-tab",
                ["enabled"] = true,
                ["count"] = 3,
                ["nested"] = new Dictionary<string, object>(StringComparer.Ordinal)
                {
                    ["brace"] = "{not-json-structure}",
                    ["empty"] = string.Empty,
                },
                ["items"] = new object[] { "α", "β", 42 },
            },
        };

        var encoded = codec.Serialize(request);
        var decoded = codec.Deserialize<RequestEnvelopeDto>(encoded);

        Assert.Equal(request.RequestId, decoded.RequestId);
        Assert.Equal("part.open", decoded.Operation);
        Assert.NotNull(decoded.Payload);
        var decodedPayload = decoded.Payload!;
        Assert.Equal(request.Payload!["path"], decodedPayload["path"]);
        Assert.Equal(request.Payload["note"], decodedPayload["note"]);
        Assert.True(Convert.ToBoolean(decodedPayload["enabled"]));

        using var doc = JsonDocument.Parse(encoded);
        var root = doc.RootElement;
        Assert.Equal(request.RequestId, root.GetProperty("request_id").GetString());
        Assert.Equal("part.open", root.GetProperty("op").GetString());
        Assert.Equal((string)request.Payload["path"], root.GetProperty("payload").GetProperty("path").GetString());
        Assert.Equal("{not-json-structure}", root.GetProperty("payload").GetProperty("nested").GetProperty("brace").GetString());
    }

    [Fact]
    public void Go_style_golden_request_deserializes_without_shape_translation()
    {
        const string golden = "{\"request_id\":\"req-123\",\"op\":\"feature.create_block\",\"payload\":{\"length\":100,\"width\":50,\"height\":25,\"boolean_op\":\"create\"}}";
        var codec = new JsonWireCodec();

        var request = codec.Deserialize<RequestEnvelopeDto>(Encoding.UTF8.GetBytes(golden));

        Assert.Equal("req-123", request.RequestId);
        Assert.Equal("feature.create_block", request.Operation);
        Assert.NotNull(request.Payload);
        var payload = request.Payload!;
        Assert.Equal(100d, Convert.ToDouble(payload["length"]));
        Assert.Equal("create", Convert.ToString(payload["boolean_op"]));

        var roundtrip = codec.Serialize(request);
        using var expectedDoc = JsonDocument.Parse(golden);
        using var actualDoc = JsonDocument.Parse(roundtrip);
        Assert.Equal(expectedDoc.RootElement.GetProperty("request_id").GetString(), actualDoc.RootElement.GetProperty("request_id").GetString());
        Assert.Equal(expectedDoc.RootElement.GetProperty("op").GetString(), actualDoc.RootElement.GetProperty("op").GetString());
        Assert.Equal(100d, actualDoc.RootElement.GetProperty("payload").GetProperty("length").GetDouble());
    }

    [Fact]
    public void Error_envelope_roundtrip_is_typed_and_omits_payload()
    {
        var codec = new JsonWireCodec();
        var response = new ResponseEnvelopeDto
        {
            RequestId = "req-error",
            Status = "ERROR",
            Error = new WireErrorDto
            {
                Category = "INVALID_ARGUMENT",
                NxErrorCode = 0,
                Message = "bad {ref} \\\"quoted\\\"",
                Recoverable = false,
                SessionHealth = "healthy",
            },
        };

        var encoded = codec.Serialize(response);
        using var doc = JsonDocument.Parse(encoded);
        Assert.False(doc.RootElement.TryGetProperty("payload", out _));
        Assert.Equal("INVALID_ARGUMENT", doc.RootElement.GetProperty("error").GetProperty("category").GetString());

        var decoded = codec.Deserialize<ResponseEnvelopeDto>(encoded);
        Assert.NotNull(decoded.Error);
        var decodedError = decoded.Error!;
        Assert.Equal(response.Error!.Message, decodedError.Message);
    }

    [Fact]
    public void Payload_size_limit_fails_closed_on_read_and_write()
    {
        var codec = new JsonWireCodec(maxPayloadBytes: 128);
        var request = new RequestEnvelopeDto
        {
            RequestId = "req-large",
            Operation = "part.open",
            Payload = new Dictionary<string, object>
            {
                ["path"] = new string('x', 512),
            },
        };

        Assert.Throws<SerializationException>(() => codec.Serialize(request));
        Assert.Throws<SerializationException>(() => codec.Deserialize<RequestEnvelopeDto>(new byte[129]));
    }
}
