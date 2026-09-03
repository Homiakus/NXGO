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
        Console.WriteLine("NXGO bootstrap: entered");
        var binDir = Environment.GetEnvironmentVariable("NXGO_AGENT_BIN");
        if (string.IsNullOrWhiteSpace(binDir))
        {
            throw new InvalidOperationException("NXGO_AGENT_BIN must point to the compiled NXGO Agent directory");
        }
        binDir = Path.GetFullPath(binDir);
        var diagnosticPath = Path.Combine(binDir, "bootstrap-diagnostics.log");
        Action<string> log = message =>
        {
            var line = DateTime.UtcNow.ToString("O") + " " + message + Environment.NewLine;
            File.AppendAllText(diagnosticPath, line);
            Console.WriteLine(message);
        };
        log("entered bin=" + binDir);

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
        log("loading dependencies");
        Assembly.LoadFrom(jsonPath);
        Assembly.LoadFrom(protocolPath);
        Assembly.LoadFrom(corePath);
        var nxOpenAssembly = typeof(Session).Assembly;
        var nxUtilitiesAssembly = typeof(NXOpen.TaggedObject).Assembly;
        AppDomain.CurrentDomain.AssemblyResolve += (sender, eventArgs) =>
        {
            var requested = new AssemblyName(eventArgs.Name).Name;
            if (string.Equals(requested, nxOpenAssembly.GetName().Name, StringComparison.OrdinalIgnoreCase)) return nxOpenAssembly;
            if (string.Equals(requested, nxUtilitiesAssembly.GetName().Name, StringComparison.OrdinalIgnoreCase)) return nxUtilitiesAssembly;
            return null;
        };
        // Load the NXHost bytes into the journal's load context. LoadFrom would
        // create a second NXOpen identity when the host is outside NX's probe
        // path, making Session appear as "NXOpen.Session" but fail type casts.
        var host = Assembly.Load(File.ReadAllBytes(hostPath));
        log("host loaded");
        log("bootstrap NXOpen=" + typeof(Session).Assembly.FullName + " @ " + typeof(Session).Assembly.Location);
        log("bootstrap TaggedObject=" + typeof(NXOpen.TaggedObject).Assembly.FullName + " @ " + typeof(NXOpen.TaggedObject).Assembly.Location);
        var entryType = host.GetType("NXGO.Agent.NXHost.EntryPoint", true);
        var identity = entryType.GetMethod("RuntimeAssemblyIdentity", BindingFlags.Public | BindingFlags.Static);
        if (identity != null) log("host identities=" + identity.Invoke(null, null));
        var main = entryType.GetMethod("Run", BindingFlags.Public | BindingFlags.Static);
        if (main == null)
        {
            throw new MissingMethodException("NXGO.Agent.NXHost.EntryPoint.Run was not found");
        }

        log("invoking EntryPoint.Run");
        try
        {
            main.Invoke(null, new object[] { args ?? new string[0] });
        }
        catch (TargetInvocationException ex)
        {
            log("target invocation failed: " + ex);
            if (ex.InnerException != null) throw ex.InnerException;
            throw;
        }
        catch (Exception ex)
        {
            log("bootstrap failed: " + ex);
            throw;
        }
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }
}
