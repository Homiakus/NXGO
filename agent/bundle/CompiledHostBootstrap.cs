using System;
using System.IO;
using System.Reflection;
using NXOpen;

/// <summary>
/// Minimal run_journal-compatible bootstrap for the canonical compiled Agent.
/// All runtime logic lives in NXGO.Agent.Core + NXGO.Agent.NXHost; this file
/// exists only because run_journal is the supported process entry used by the
/// dedicated-worker supervisor.
/// </summary>
public static class NXGOCompiledHostBootstrap
{
    public static void Main(string[] args)
    {
        var binDir = Environment.GetEnvironmentVariable("NXGO_AGENT_BIN");
        if (string.IsNullOrWhiteSpace(binDir))
        {
            throw new InvalidOperationException("NXGO_AGENT_BIN must point to the compiled NXGO Agent directory");
        }
        binDir = Path.GetFullPath(binDir);

        var corePath = Path.Combine(binDir, "NXGO.Agent.Core.dll");
        var hostPath = Path.Combine(binDir, "NXGO.Agent.NXHost.dll");
        if (!File.Exists(corePath)) throw new FileNotFoundException("NXGO.Agent.Core.dll not found", corePath);
        if (!File.Exists(hostPath)) throw new FileNotFoundException("NXGO.Agent.NXHost.dll not found", hostPath);

        // Load Core first so the NXHost ProjectReference resolves deterministically
        // from the exact directory selected by the supervisor/build gate.
        Assembly.LoadFrom(corePath);
        var host = Assembly.LoadFrom(hostPath);
        var entryType = host.GetType("NXGO.Agent.NXHost.EntryPoint", true);
        var main = entryType.GetMethod("Main", BindingFlags.Public | BindingFlags.Static);
        if (main == null)
        {
            throw new MissingMethodException("NXGO.Agent.NXHost.EntryPoint.Main was not found");
        }

        try
        {
            main.Invoke(null, new object[] { args ?? new string[0] });
        }
        catch (TargetInvocationException ex)
        {
            if (ex.InnerException != null) throw ex.InnerException;
            throw;
        }
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }
}
