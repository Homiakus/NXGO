using System.Text;
using System.Text.Json.Nodes;
using NXGO.Protocol;
using Xunit;

namespace NXGO.Protocol.Tests;

public sealed class CrossLanguageGoldenTests
{
    private readonly JsonWireCodec _codec = new JsonWireCodec();

    [Fact]
    public void Shared_handshake_v2_matches_complete_Go_contract()
    {
        var golden = ReadGolden("handshake_request_v2.json");
        var msg = _codec.Deserialize<HandshakeRequestDto>(golden);

        Assert.Equal(2, msg.ProtocolVersion.Major);
        Assert.Equal(0, msg.ProtocolVersion.Minor);
        Assert.Equal("dedicated-worker", msg.RequestedMode);
        Assert.Equal(new[] { "geometry", "assembly", "drafting" }, msg.RequestedFeatures);
        Assert.Equal("инженер-日本語", msg.ClientUser);
        Assert.Equal("nonce-crosslang-001", msg.Nonce);
        AssertSemanticRoundTrip(golden, _codec.Serialize(msg));
    }

    [Fact]
    public void Shared_extended_request_v2_preserves_metadata_unicode_paths_and_dollar_type_as_data()
    {
        var golden = ReadGolden("request_extended_v2.json");
        var req = _codec.Deserialize<RequestEnvelopeDto>(golden);

        Assert.Equal("req-русский-日本語-001", req.RequestId);
        Assert.Equal("corr-42", req.CorrelationId);
        Assert.Equal("run-crosslang", req.RunId);
        Assert.Equal("test-wire-v2", req.TestId);
        Assert.Equal("part.open", req.Operation);
        Assert.Equal(15000, req.TimeoutMs);
        Assert.Equal("tx-optional-context", req.TxId);
        Assert.NotNull(req.TraceMeta);
        Assert.Equal("cross-language", req.TraceMeta!["suite"]);

        Assert.NotNull(req.Payload);
        Assert.Equal("C:\\NXGO\\тест\\部品 {draft} \"quoted\".prt", req.Payload!["path"]);
        var attack = Assert.IsType<Dictionary<string, object>>(req.Payload["attack"]);
        Assert.Equal("System.IO.FileInfo, System.IO.FileSystem", attack["$type"]);
        Assert.Equal("must-remain-data", attack["value"]);
        Assert.DoesNotContain(req.Payload.Values, value => value is Type);

        AssertSemanticRoundTrip(golden, _codec.Serialize(req));
    }

    [Fact]
    public void Shared_error_response_v2_preserves_full_diagnostics_and_timing()
    {
        var golden = ReadGolden("response_error_v2.json");
        var resp = _codec.Deserialize<ResponseEnvelopeDto>(golden);

        Assert.Equal("ERROR", resp.Status);
        Assert.NotNull(resp.Error);
        Assert.Equal("INVALID_ARGUMENT", resp.Error!.Category);
        Assert.Equal("part.open", resp.Error.Operation);
        Assert.Equal("corr-42", resp.Error.CorrelationId);
        Assert.Equal("shared cross-language fixture", resp.Error.Diagnostic);
        Assert.Equal(new[] { "warning α", "warning β" }, resp.Warnings);
        Assert.NotNull(resp.Timing);
        Assert.Equal(6, resp.Timing!.TotalDurationMs);

        AssertSemanticRoundTrip(golden, _codec.Serialize(resp));
    }

    [Fact]
    public void Shared_handle_response_v2_preserves_generation_and_lifetime_fields()
    {
        var golden = ReadGolden("response_handles_v2.json");
        var resp = _codec.Deserialize<ResponseEnvelopeDto>(golden);

        Assert.Equal("OK", resp.Status);
        Assert.NotNull(resp.ProducedHandles);
        Assert.Equal(2, resp.ProducedHandles!.Length);
        Assert.Equal((uint)3, resp.ProducedHandles[0].Generation);
        Assert.Equal("Feature", resp.ProducedHandles[0].Kind);
        Assert.Equal("req-handles-001", resp.ProducedHandles[0].LeaseScopeId);
        Assert.Equal((uint)5, resp.ProducedHandles[1].Generation);
        Assert.Equal((uint)102, resp.ProducedHandles[1].NativeTag);
        Assert.NotNull(resp.Timing);
        Assert.Equal(14, resp.Timing!.TotalDurationMs);

        AssertSemanticRoundTrip(golden, _codec.Serialize(resp));
    }

    [Fact]
    public void Deeply_nested_payload_exceeding_codec_depth_fails_closed()
    {
        var nested = new StringBuilder();
        nested.Append("{\"request_id\":\"req-depth\",\"op\":\"nx.ping\",\"payload\":");
        for (var i = 0; i < 80; i++) nested.Append("{\"x\":");
        nested.Append("1");
        for (var i = 0; i < 80; i++) nested.Append('}');
        nested.Append('}');

        Assert.ThrowsAny<Exception>(() => _codec.Deserialize<RequestEnvelopeDto>(Encoding.UTF8.GetBytes(nested.ToString())));
    }

    private static byte[] ReadGolden(string name)
    {
        var root = FindRepoRoot();
        return File.ReadAllBytes(Path.Combine(root, "testdata", "protocol", name));
    }

    private static string FindRepoRoot()
    {
        var dir = new DirectoryInfo(AppContext.BaseDirectory);
        while (dir != null)
        {
            if (File.Exists(Path.Combine(dir.FullName, "go.mod"))) return dir.FullName;
            dir = dir.Parent;
        }
        throw new DirectoryNotFoundException("could not find NXGO repository root from test output directory");
    }

    private static void AssertSemanticRoundTrip(byte[] expected, byte[] actual)
    {
        var expectedNode = JsonNode.Parse(expected);
        var actualNode = JsonNode.Parse(actual);
        Assert.True(JsonNode.DeepEquals(expectedNode, actualNode),
            $"semantic JSON mismatch\nexpected: {Encoding.UTF8.GetString(expected)}\nactual:   {Encoding.UTF8.GetString(actual)}");
    }
}
