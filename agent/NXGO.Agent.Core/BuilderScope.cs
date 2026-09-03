using System.Threading;

namespace NXGO.Agent.Core;

/// <summary>
/// Owns one NX builder instance. A commit may be attempted at most once.
/// Disposal always invokes the provided destroy action.
/// </summary>
public sealed class BuilderScope<TBuilder> : IDisposable where TBuilder : class
{
    private static int _activeCount;
    private static int _highWatermark;
    private readonly Action<TBuilder> _destroy;
    private bool _commitAttempted;
    private bool _disposed;

    public BuilderScope(TBuilder builder, Action<TBuilder> destroy)
    {
        Builder = builder ?? throw new ArgumentNullException(nameof(builder));
        _destroy = destroy ?? throw new ArgumentNullException(nameof(destroy));
        var active = Interlocked.Increment(ref _activeCount);
        while (true)
        {
            var observed = Volatile.Read(ref _highWatermark);
            if (active <= observed || Interlocked.CompareExchange(ref _highWatermark, active, observed) == observed) break;
        }
    }

    public TBuilder Builder { get; }

    public static int ActiveCount => Volatile.Read(ref _activeCount);

    public static int HighWatermark => Volatile.Read(ref _highWatermark);

    public TResult CommitOnce<TResult>(Func<TBuilder, TResult> commit)
    {
        if (commit is null) throw new ArgumentNullException(nameof(commit));
        ThrowIfDisposed();
        if (_commitAttempted)
        {
            throw new InvalidOperationException("Builder commit already attempted; create a fresh builder for retry.");
        }

        _commitAttempted = true;
        return commit(Builder);
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Interlocked.Decrement(ref _activeCount);
        _destroy(Builder);
    }

    private void ThrowIfDisposed()
    {
        if (_disposed) throw new ObjectDisposedException(nameof(BuilderScope<TBuilder>));
    }
}
