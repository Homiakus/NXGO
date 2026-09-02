using System.Collections.Concurrent;
using System.Threading;

namespace NXGO.Agent.Core;

/// <summary>
/// Observable lifecycle for a queued NX work item. The distinction between
/// Queued/CancelledBeforeStart and Started is safety-critical: once work has
/// entered NX execution, caller cancellation can no longer be interpreted as
/// proof that the CAD operation did not run.
/// </summary>
public enum NxExecutionState
{
    Queued = 0,
    Started = 1,
    Completed = 2,
    Faulted = 3,
    CancelledBeforeStart = 4,
}

/// <summary>
/// Tracked execution returned by <see cref="NxExecutor.EnqueueTracked{TResult}"/>.
/// </summary>
public sealed class NxExecution<TResult>
{
    private readonly TaskCompletionSource<TResult> _completion =
        new TaskCompletionSource<TResult>(TaskCreationOptions.RunContinuationsAsynchronously);
    private int _state = (int)NxExecutionState.Queued;

    public Task<TResult> Task => _completion.Task;

    public NxExecutionState State => (NxExecutionState)Volatile.Read(ref _state);

    /// <summary>
    /// Cancels only while the work item is still provably queued. Returns false
    /// after NX execution has started, which signals that the caller must wait
    /// for a final result or treat a subsequent transport loss as ambiguous.
    /// </summary>
    public bool TryCancelBeforeStart()
    {
        if (Interlocked.CompareExchange(
                ref _state,
                (int)NxExecutionState.CancelledBeforeStart,
                (int)NxExecutionState.Queued) != (int)NxExecutionState.Queued)
        {
            return false;
        }

        _completion.TrySetCanceled();
        return true;
    }

    internal bool TryMarkStarted()
    {
        return Interlocked.CompareExchange(
                   ref _state,
                   (int)NxExecutionState.Started,
                   (int)NxExecutionState.Queued) == (int)NxExecutionState.Queued;
    }

    internal void Complete(TResult result)
    {
        if (Interlocked.CompareExchange(
                ref _state,
                (int)NxExecutionState.Completed,
                (int)NxExecutionState.Started) != (int)NxExecutionState.Started)
        {
            throw new InvalidOperationException($"cannot complete NX work from state {State}");
        }
        _completion.TrySetResult(result);
    }

    internal void Fail(Exception exception)
    {
        if (exception is null) throw new ArgumentNullException(nameof(exception));
        if (Interlocked.CompareExchange(
                ref _state,
                (int)NxExecutionState.Faulted,
                (int)NxExecutionState.Started) != (int)NxExecutionState.Started)
        {
            throw new InvalidOperationException($"cannot fault NX work from state {State}");
        }
        _completion.TrySetException(exception);
    }
}

/// <summary>
/// Queue/gateway that prevents transport/background threads from executing NX work directly.
/// The NX host MUST bind this executor on the supported NX execution thread and call Drain* there.
/// </summary>
public sealed class NxExecutor
{
    private readonly ConcurrentQueue<IWorkItem> _queue = new ConcurrentQueue<IWorkItem>();
    private int? _boundThreadId;

    public int PendingCount => _queue.Count;

    public void BindToCurrentThread()
    {
        var current = Environment.CurrentManagedThreadId;
        if (_boundThreadId.HasValue && _boundThreadId.Value != current)
        {
            throw new InvalidOperationException($"NxExecutor already bound to thread {_boundThreadId.Value}; cannot bind to {current}.");
        }
        _boundThreadId = current;
    }

    public Task<TResult> Enqueue<TResult>(Func<TResult> operation, CancellationToken cancellationToken = default(CancellationToken))
    {
        return EnqueueTracked(operation, cancellationToken).Task;
    }

    public NxExecution<TResult> EnqueueTracked<TResult>(Func<TResult> operation, CancellationToken cancellationToken = default(CancellationToken))
    {
        if (operation is null) throw new ArgumentNullException(nameof(operation));
        var execution = new NxExecution<TResult>();
        var item = new WorkItem<TResult>(operation, execution, cancellationToken);
        _queue.Enqueue(item);
        return execution;
    }

    public bool DrainOne()
    {
        RequireBoundThread();
        if (!_queue.TryDequeue(out var item)) return false;
        item.Execute();
        return true;
    }

    public int DrainUntilEmpty(int maxItems = 1024)
    {
        if (maxItems <= 0) throw new ArgumentOutOfRangeException(nameof(maxItems));
        RequireBoundThread();
        var drained = 0;
        while (drained < maxItems && _queue.TryDequeue(out var item))
        {
            item.Execute();
            drained++;
        }
        return drained;
    }

    private void RequireBoundThread()
    {
        if (!_boundThreadId.HasValue)
        {
            throw new InvalidOperationException("NxExecutor is not bound to an NX execution thread.");
        }
        var current = Environment.CurrentManagedThreadId;
        if (_boundThreadId.Value != current)
        {
            throw new InvalidOperationException($"NX work drain attempted on thread {current}; expected {_boundThreadId.Value}.");
        }
    }

    private interface IWorkItem
    {
        void Execute();
    }

    private sealed class WorkItem<TResult> : IWorkItem
    {
        private readonly Func<TResult> _operation;
        private readonly NxExecution<TResult> _execution;
        private readonly CancellationToken _cancellationToken;

        public WorkItem(
            Func<TResult> operation,
            NxExecution<TResult> execution,
            CancellationToken cancellationToken)
        {
            _operation = operation;
            _execution = execution;
            _cancellationToken = cancellationToken;
        }

        public void Execute()
        {
            if (_cancellationToken.IsCancellationRequested)
            {
                _execution.TryCancelBeforeStart();
                return;
            }

            // A caller may explicitly cancel the tracked execution before the
            // NX thread reaches it. In that case it remains in the lock-free
            // queue as a harmless tombstone and is skipped here.
            if (!_execution.TryMarkStarted())
            {
                return;
            }

            try
            {
                _execution.Complete(_operation());
            }
            catch (Exception ex)
            {
                _execution.Fail(ex);
            }
        }
    }
}
