using System.Diagnostics;
using System.Text;
using NXGO.Agent.Core;
using NXOpen;

namespace NXGO.Agent.NXHost;

/// <summary>
/// Dedicated-worker bootstrap. Main stays on the NX execution thread and drains NxExecutor.
/// This host is intentionally not the interactive-attach implementation.
/// </summary>
public static class EntryPoint
{
    private static readonly SessionHealthState Health = new SessionHealthState();
    private static volatile bool _shutdownRequested;

    public static void Main(string[] args)
    {
        var session = Session.GetSession();
        var executor = new NxExecutor();
        executor.BindToCurrentThread();

        var pipeName = Environment.GetEnvironmentVariable("NXGO_PIPE_NAME");
        if (string.IsNullOrWhiteSpace(pipeName))
        {
            pipeName = "nxgo-worker-" + Process.GetCurrentProcess().Id;
        }

        using (var server = new NamedPipeRequestServer(pipeName!, (payload, token) => HandleRequest(session, executor, payload, token)))
        {
            session.LogFile.WriteLine($"[NXGO] worker agent start pipe={pipeName}");
            server.Start();

            while (!_shutdownRequested && Health.Value != SessionHealth.Lost)
            {
                executor.DrainUntilEmpty(64);
                Thread.Sleep(5);
            }

            session.LogFile.WriteLine($"[NXGO] worker agent stop health={Health.Value}");
        }
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }

    private static Task<byte[]> HandleRequest(Session session, NxExecutor executor, byte[] payload, CancellationToken token)
    {
        var request = Encoding.UTF8.GetString(payload);
        switch (request)
        {
            case "ping":
                return Task.FromResult(Encoding.UTF8.GetBytes("ok|pong"));

            case "nx.ping":
                return Map(executor.Enqueue(() =>
                {
                    Health.RequireReusable();
                    session.LogFile.WriteLine("[NXGO] nx.ping");
                    return Encoding.UTF8.GetBytes("ok|nx.pong");
                }, token));

            case "shutdown":
                _shutdownRequested = true;
                return Task.FromResult(Encoding.UTF8.GetBytes("ok|shutdown"));

            default:
                return Task.FromResult(Encoding.UTF8.GetBytes("error|unsupported_operation"));
        }
    }

    private static async Task<byte[]> Map(Task<byte[]> task)
    {
        try
        {
            return await task.ConfigureAwait(false);
        }
        catch (Exception ex)
        {
            return Encoding.UTF8.GetBytes("error|" + ex.GetType().Name + "|" + ex.Message.Replace('|', '/'));
        }
    }
}
