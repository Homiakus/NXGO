using System.Globalization;
using System.Runtime.Serialization;
using System.Text;
using Newtonsoft.Json;

namespace NXGO.Protocol;

/// <summary>
/// Canonical Siemens-independent JSON codec for the NXGO wire contract.
/// Type metadata is explicitly disabled: payload JSON is data, never a request
/// to instantiate arbitrary CLR types. The same netstandard2.0 codec is used by
/// ordinary CI and the net48 NX host.
/// </summary>
public sealed class JsonWireCodec
{
    public const int DefaultMaxPayloadBytes = 4 * 1024 * 1024;

    private static readonly JsonSerializerSettings Settings = new JsonSerializerSettings
    {
        Formatting = Formatting.None,
        TypeNameHandling = TypeNameHandling.None,
        MetadataPropertyHandling = MetadataPropertyHandling.Ignore,
        DateParseHandling = DateParseHandling.None,
        FloatParseHandling = FloatParseHandling.Double,
        Culture = CultureInfo.InvariantCulture,
        MaxDepth = 64,
        MissingMemberHandling = MissingMemberHandling.Ignore,
        NullValueHandling = NullValueHandling.Include,
        StringEscapeHandling = StringEscapeHandling.Default,
    };

    private readonly int _maxPayloadBytes;

    public JsonWireCodec(int maxPayloadBytes = DefaultMaxPayloadBytes)
    {
        if (maxPayloadBytes <= 0) throw new ArgumentOutOfRangeException(nameof(maxPayloadBytes));
        _maxPayloadBytes = maxPayloadBytes;
    }

    public int MaxPayloadBytes => _maxPayloadBytes;

    public byte[] Serialize<T>(T value)
    {
        if (value == null) throw new ArgumentNullException(nameof(value));
        var json = JsonConvert.SerializeObject(value, Settings);
        var encoded = Encoding.UTF8.GetBytes(json);
        if (encoded.Length > _maxPayloadBytes)
        {
            throw new SerializationException($"serialized payload exceeds {_maxPayloadBytes} bytes");
        }
        return encoded;
    }

    public T Deserialize<T>(byte[] payload)
    {
        if (payload == null) throw new ArgumentNullException(nameof(payload));
        if (payload.Length > _maxPayloadBytes)
        {
            throw new SerializationException($"wire payload exceeds {_maxPayloadBytes} bytes");
        }

        var json = Encoding.UTF8.GetString(payload);
        var value = JsonConvert.DeserializeObject<T>(json, Settings);
        if (value == null)
        {
            throw new SerializationException("wire payload did not deserialize to " + typeof(T).FullName);
        }
        return value;
    }

    public string SerializeUtf8<T>(T value)
    {
        return Encoding.UTF8.GetString(Serialize(value));
    }
}
