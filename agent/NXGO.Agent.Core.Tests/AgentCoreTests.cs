using System.IO.Pipes;
using System.Text;
using NXGO.Agent.Core;

namespace NXGO.Agent.Core.Tests;

public sealed class AgentCoreTests
{
    [Fact]
    public void BuilderScope_destroys_after_success()
    {
        var builder = new FakeBuilder();
        using (var scope = new BuilderScope<FakeBuilder>(builder, b => b.Destroyed++))
        {
            var value = scope.CommitOnce(b => b.Commit());
            Assert.Equal(1, value);
        }
        Assert.Equal(1, builder.Destroyed);
    }

    [Fact]
    public void BuilderScope_destroys_after_commit_failure_and_rejects_retry()
    {
        var builder = new FakeBuilder { Fail = true };
        var scope = new BuilderScope<FakeBuilder>(builder, b => b.Destroyed++);
        Assert.Throws<InvalidOperationException>(() => scope.CommitOnce(b => b.Commit()));
        Assert.Throws<InvalidOperationException>(() => scope.CommitOnce(b => b.Commit()));
        scope.Dispose();
        Assert.Equal(1, builder.Destroyed);
    }

    [Fact]
    public void NxExecutor_executes_only_when_bound_thread_drains()
    {
        var executor = new NxExecutor();
        executor.BindToCurrentThread();
        var boundThread = Environment.CurrentManagedThreadId;
        Task<int>? queuedTask = null;
        using var queued = new ManualResetEventSlim(false);
        var producer = new Thread(() =>
        {
            queuedTask = executor.Enqueue(() => Environment.CurrentManagedThreadId);
            queued.Set();
        });
        producer.Start();
        Assert.True(queued.Wait(TimeSpan.FromSeconds(5)));
        producer.Join();

        Assert.NotNull(queuedTask);
        Assert.False(queuedTask!.IsCompleted);
        Assert.True(executor.DrainOne());
        Assert.Equal(boundThread, queuedTask.GetAwaiter().GetResult());
    }

    [Fact]
    public void NxExecutor_rejects_drain_from_foreign_thread()
    {
        var executor = new NxExecutor();
        executor.BindToCurrentThread();
        Exception? observed = null;
        var worker = new Thread(() =>
        {
            try { executor.DrainOne(); }
            catch (Exception ex) { observed = ex; }
        });
        worker.Start();
        worker.Join();
        Assert.IsType<InvalidOperationException>(observed);
    }

    [Fact]
    public void NxExecutor_cancels_queued_work_before_execution()
    {
        var executor = new NxExecutor();
        executor.BindToCurrentThread();
        using var cts = new CancellationTokenSource();
        var task = executor.Enqueue(() => 42, cts.Token);
        cts.Cancel();
        executor.DrainOne();
        Assert.ThrowsAny<OperationCanceledException>(() => task.GetAwaiter().GetResult());
    }

    [Fact]
    public void SessionHealth_terminal_states_are_not_reusable()
    {
        var health = new SessionHealthState();
        Assert.True(health.IsReusable);
        health.MarkSuspect();
        Assert.False(health.IsReusable);
        health.MarkPoisoned();
        Assert.False(health.IsReusable);
        Assert.Throws<InvalidOperationException>(() => health.RequireReusable());
    }

    [Fact]
    public void FrameCodec_round_trips_and_rejects_mismatch()
    {
        var payload = Encoding.UTF8.GetBytes("nxgo");
        var frame = FrameCodec.Encode(payload);
        Assert.Equal(payload, FrameCodec.Decode(frame));
        Array.Resize(ref frame, frame.Length - 1);
        Assert.Throws<InvalidDataException>(() => FrameCodec.Decode(frame));
    }

    [Fact]
    public async Task NamedPipeServer_round_trips_without_NX()
    {
        var pipeName = "nxgo-test-" + Guid.NewGuid().ToString("N");
        using var server = new NamedPipeRequestServer(pipeName, (payload, _) =>
            Task.FromResult(Encoding.UTF8.GetBytes(Encoding.UTF8.GetString(payload).ToUpperInvariant())));
        server.Start();

        using var client = new NamedPipeClientStream(".", pipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(5));
        await client.ConnectAsync(timeout.Token);

        var frame = FrameCodec.Encode(Encoding.UTF8.GetBytes("ping"));
        await client.WriteAsync(frame, timeout.Token);
        await client.FlushAsync(timeout.Token);

        var header = new byte[4];
        await ReadExactAsync(client, header, timeout.Token);
        var length = header[0] | (header[1] << 8) | (header[2] << 16) | (header[3] << 24);
        var payload = new byte[length];
        await ReadExactAsync(client, payload, timeout.Token);
        Assert.Equal("PING", Encoding.UTF8.GetString(payload));
    }

    private static async Task ReadExactAsync(Stream stream, byte[] buffer, CancellationToken token)
    {
        var offset = 0;
        while (offset < buffer.Length)
        {
            var read = await stream.ReadAsync(buffer.AsMemory(offset), token);
            if (read == 0) throw new EndOfStreamException();
            offset += read;
        }
    }

    private sealed class FakeBuilder
    {
        public bool Fail { get; set; }
        public int Destroyed { get; set; }

        public int Commit()
        {
            if (Fail) throw new InvalidOperationException("commit failed");
            return 1;
        }
    }
}
