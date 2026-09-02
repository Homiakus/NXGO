using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace NXGO.Protocol;

/// <summary>
/// Json.NET normally materializes values declared as object as JObject/JArray.
/// NXGO intentionally exposes a serializer-neutral payload model instead:
/// Dictionary&lt;string,object&gt;, object[], and CLR primitive values. This keeps
/// the protocol layer independent from Json.NET types and lets NXHost migrate
/// serializers without rewriting every domain adapter in the same change.
/// </summary>
internal sealed class PlainObjectJsonConverter : JsonConverter
{
    public override bool CanWrite => false;

    public override bool CanConvert(Type objectType)
    {
        return objectType == typeof(object);
    }

    public override object? ReadJson(JsonReader reader, Type objectType, object? existingValue, JsonSerializer serializer)
    {
        if (reader.TokenType == JsonToken.Null) return null;
        var token = JToken.Load(reader);
        return ToPlainValue(token);
    }

    public override void WriteJson(JsonWriter writer, object? value, JsonSerializer serializer)
    {
        throw new NotSupportedException("PlainObjectJsonConverter is read-only");
    }

    private static object? ToPlainValue(JToken token)
    {
        switch (token.Type)
        {
            case JTokenType.Object:
            {
                var result = new Dictionary<string, object>(StringComparer.Ordinal);
                foreach (var property in ((JObject)token).Properties())
                {
                    var value = ToPlainValue(property.Value);
                    result[property.Name] = value!;
                }
                return result;
            }
            case JTokenType.Array:
                return ((JArray)token).Select(ToPlainValue).ToArray();
            case JTokenType.Integer:
            case JTokenType.Float:
            case JTokenType.String:
            case JTokenType.Boolean:
            case JTokenType.Date:
            case JTokenType.Bytes:
            case JTokenType.Guid:
            case JTokenType.Uri:
            case JTokenType.TimeSpan:
                return ((JValue)token).Value;
            case JTokenType.Null:
            case JTokenType.Undefined:
                return null;
            default:
                throw new JsonSerializationException("unsupported JSON token in NXGO payload: " + token.Type);
        }
    }
}
