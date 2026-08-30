using System.Collections.Concurrent;

namespace NXGO.Agent.Core;

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
        if (operation is null) throw new ArgumentNullException(nameof(operation));
        var item = new WorkItem<TResult>(operation, cancellationToken);
        _queue.Enqueue(item);
        return item.Task;
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
        private readonly CancellationToken _cancellationToken;
        private readonly TaskCompletionSource<TResult> _completion =
            new TaskCompletionSource<TResult>(TaskCreationOptions.RunContinuationsAsynchronously);

        public WorkItem(Func<TResult> operation, CancellationToken cancellationToken)
        {
            _operation = operation;
            _cancellationToken = cancellationToken;
        }

        public Task<TResult> Task => _completion.Task;

        public void Execute()
        {
            if (_cancellationToken.IsCancellationRequested)
            {
                _completion.TrySetCanceled();
                return;
            }

            try
            {
                _completion.TrySetResult(_operation());
            }
            catch (Exception ex)
            {
                _completion.TrySetException(ex);
            }
        }
    }
}
