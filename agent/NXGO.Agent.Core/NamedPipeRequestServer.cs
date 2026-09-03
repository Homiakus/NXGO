using System.IO.Pipes;

namespace NXGO.Agent.Core;

/// <summary>
/// Length-prefixed local named-pipe server. The handler MUST NOT call NXOpen directly;
/// NX-dependent handlers enqueue work through <see cref="NxExecutor"/>.
/// </summary>
public sealed class NamedPipeRequestServer : IDisposable
{
    private readonly string _pipeName;
    private readonly Func<byte[], CancellationToken, Task<byte[]>> _handler;
    private readonly Action? _onDisconnected;
    private readonly CancellationTokenSource _shutdown = new CancellationTokenSource();
    private Task? _loop;
    private NamedPipeServerStream? _activeServer;

    public NamedPipeRequestServer(string pipeName, Func<byte[], CancellationToken, Task<byte[]>> handler, Action? onDisconnected = null)
    {
        if (string.IsNullOrWhiteSpace(pipeName)) throw new ArgumentException("pipe name is required", nameof(pipeName));
        _pipeName = pipeName;
        _handler = handler ?? throw new ArgumentNullException(nameof(handler));
        _onDisconnected = onDisconnected;
    }

    public Task Completion => _loop ?? Task.CompletedTask;

    public void Start()
    {
        if (_loop != null) throw new InvalidOperationException("server already started");
        _loop = Task.Run(() => RunLoop(_shutdown.Token));
    }

    public void Dispose()
    {
        _shutdown.Cancel();
        try { _activeServer?.Dispose(); } catch { }
        try { _loop?.Wait(TimeSpan.FromSeconds(2)); } catch { }
        _shutdown.Dispose();
    }

    private void RunLoop(CancellationToken cancellationToken)
    {
        while (!cancellationToken.IsCancellationRequested)
        {
            using (var server = new NamedPipeServerStream(
                _pipeName,
                PipeDirection.InOut,
                1,
                PipeTransmissionMode.Byte,
                PipeOptions.Asynchronous))
            {
                _activeServer = server;
                try
                {
                    server.WaitForConnection();
                    ServeConnection(server, cancellationToken);
                }
                catch (ObjectDisposedException) when (cancellationToken.IsCancellationRequested)
                {
                    return;
                }
                catch (IOException) when (cancellationToken.IsCancellationRequested)
                {
                    return;
                }
                finally
                {
                    _activeServer = null;
                    try { _onDisconnected?.Invoke(); } catch { }
                }
            }
        }
    }

    private void ServeConnection(Stream stream, CancellationToken cancellationToken)
    {
        while (!cancellationToken.IsCancellationRequested)
        {
            var header = new byte[FrameCodec.HeaderSize];
            if (!TryReadExact(stream, header, cancellationToken)) return;
            var length = header[0] | (header[1] << 8) | (header[2] << 16) | (header[3] << 24);
            if (length < 0 || length > FrameCodec.DefaultMaxPayloadBytes)
            {
                throw new InvalidDataException("invalid request frame length");
            }

            var payload = new byte[length];
            if (length > 0 && !TryReadExact(stream, payload, cancellationToken)) return;

            var responsePayload = _handler(payload, cancellationToken).GetAwaiter().GetResult();
            var responseFrame = FrameCodec.Encode(responsePayload);
            stream.Write(responseFrame, 0, responseFrame.Length);
            stream.Flush();
        }
    }

    private static bool TryReadExact(Stream stream, byte[] buffer, CancellationToken cancellationToken)
    {
        var offset = 0;
        while (offset < buffer.Length)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var read = stream.Read(buffer, offset, buffer.Length - offset);
            if (read == 0) return false;
            offset += read;
        }
        return true;
    }
}
