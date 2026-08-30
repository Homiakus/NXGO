namespace NXGO.Agent.Core;

public static class FrameCodec
{
    public const int HeaderSize = 4;
    public const int DefaultMaxPayloadBytes = 4 * 1024 * 1024;

    public static byte[] Encode(byte[] payload)
    {
        if (payload is null) throw new ArgumentNullException(nameof(payload));
        if (payload.Length > DefaultMaxPayloadBytes)
        {
            throw new InvalidDataException($"payload exceeds {DefaultMaxPayloadBytes} bytes");
        }

        var frame = new byte[HeaderSize + payload.Length];
        WriteInt32LittleEndian(frame, 0, payload.Length);
        Buffer.BlockCopy(payload, 0, frame, HeaderSize, payload.Length);
        return frame;
    }

    public static byte[] Decode(byte[] frame, int maxPayloadBytes = DefaultMaxPayloadBytes)
    {
        if (frame is null) throw new ArgumentNullException(nameof(frame));
        if (frame.Length < HeaderSize) throw new InvalidDataException("truncated frame header");
        var length = ReadInt32LittleEndian(frame, 0);
        if (length < 0 || length > maxPayloadBytes) throw new InvalidDataException("invalid frame length");
        if (frame.Length != HeaderSize + length) throw new InvalidDataException("frame length mismatch");
        var payload = new byte[length];
        Buffer.BlockCopy(frame, HeaderSize, payload, 0, length);
        return payload;
    }

    private static void WriteInt32LittleEndian(byte[] buffer, int offset, int value)
    {
        buffer[offset] = (byte)value;
        buffer[offset + 1] = (byte)(value >> 8);
        buffer[offset + 2] = (byte)(value >> 16);
        buffer[offset + 3] = (byte)(value >> 24);
    }

    private static int ReadInt32LittleEndian(byte[] buffer, int offset)
    {
        return buffer[offset]
            | (buffer[offset + 1] << 8)
            | (buffer[offset + 2] << 16)
            | (buffer[offset + 3] << 24);
    }
}
