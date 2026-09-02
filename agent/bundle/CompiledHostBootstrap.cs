using System;
using System.IO;
using System.Reflection;
using NXOpen;

/// <summary>
/// Minimal run_journal-compatible bootstrap for the canonical compiled Agent.
/// All runtime logic lives in NXGO.Protocol + NXGO.Agent.Core + NXGO.Agent.NXHost;
/// this file exists only because run_journal is the supported process entry used
/// by the dedicated-worker supervisor.
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

        var jsonPath = Path.Combine(binDir, "Newtonsoft.Json.dll");
        var protocolPath = Path.Combine(binDir, "NXGO.Protocol.dll");
        var corePath = Path.Combine(binDir, "NXGO.Agent.Core.dll");
        var hostPath = Path.Combine(binDir, "NXGO.Agent.NXHost.dll");
        if (!File.Exists(jsonPath)) throw new FileNotFoundException("Newtonsoft.Json.dll not found", jsonPath);
        if (!File.Exists(protocolPath)) throw new FileNotFoundException("NXGO.Protocol.dll not found", protocolPath);
        if (!File.Exists(corePath)) throw new FileNotFoundException("NXGO.Agent.Core.dll not found", corePath);
        if (!File.Exists(hostPath)) throw new FileNotFoundException("NXGO.Agent.NXHost.dll not found", hostPath);

        // Load exact dependencies from the supervisor-selected Agent directory.
        // Json.NET is loaded before NXGO.Protocol; Protocol and Core are loaded
        // before NXHost so no ambient/global assembly can silently satisfy them.
        Assembly.LoadFrom(jsonPath);
        Assembly.LoadFrom(protocolPath);
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
