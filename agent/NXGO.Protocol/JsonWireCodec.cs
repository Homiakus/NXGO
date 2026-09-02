using System.Runtime.Serialization;
using System.Runtime.Serialization.Json;
using System.Text;

namespace NXGO.Protocol;

/// <summary>
/// Canonical Siemens-independent JSON codec for the NXGO wire contract.
/// The serializer is available to both netstandard2.0 Agent code and ordinary
/// CI, so escaping/Unicode/shape behavior can be proven without NXOpen.dll.
/// </summary>
public sealed class JsonWireCodec
{
    public const int DefaultMaxPayloadBytes = 4 * 1024 * 1024;

    private static readonly Type[] KnownPayloadTypes =
    {
        typeof(Dictionary<string, object>),
        typeof(List<object>),
        typeof(object[]),
        typeof(string[]),
        typeof(bool[]),
        typeof(int[]),
        typeof(long[]),
        typeof(double[]),
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
        var serializer = CreateSerializer(typeof(T));
        using (var stream = new MemoryStream())
        {
            serializer.WriteObject(stream, value);
            if (stream.Length > _maxPayloadBytes)
            {
                throw new SerializationException($"serialized payload exceeds {_maxPayloadBytes} bytes");
            }
            return stream.ToArray();
        }
    }

    public T Deserialize<T>(byte[] payload)
    {
        if (payload == null) throw new ArgumentNullException(nameof(payload));
        if (payload.Length > _maxPayloadBytes)
        {
            throw new SerializationException($"wire payload exceeds {_maxPayloadBytes} bytes");
        }

        var serializer = CreateSerializer(typeof(T));
        using (var stream = new MemoryStream(payload, writable: false))
        {
            var value = serializer.ReadObject(stream);
            if (!(value is T typed))
            {
                throw new SerializationException("wire payload did not deserialize to " + typeof(T).FullName);
            }
            return typed;
        }
    }

    public string SerializeUtf8<T>(T value)
    {
        return Encoding.UTF8.GetString(Serialize(value));
    }

    private static DataContractJsonSerializer CreateSerializer(Type type)
    {
        return new DataContractJsonSerializer(type, new DataContractJsonSerializerSettings
        {
            UseSimpleDictionaryFormat = true,
            MaxItemsInObjectGraph = 65_536,
            KnownTypes = KnownPayloadTypes,
        });
    }
}
