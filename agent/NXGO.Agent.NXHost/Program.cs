using System;
using System.IO;

namespace NXGO.Agent.NXHost;

/// <summary>
/// Direct .NET entrypoint for Siemens' managed_core runner. The runner owns
/// NXOpen assembly loading; this executable must not load or invoke NXHost via
/// reflection from another NX load context.
/// </summary>
public static class Program
{
    public static int Main(string[] args)
    {
        WriteDiagnostic("NXGO managed_core bootstrap: entered");
        try
        {
            EntryPoint.Run(args ?? Array.Empty<string>());
            WriteDiagnostic("NXGO managed_core bootstrap: completed");
            return 0;
        }
        catch (Exception ex)
        {
            WriteDiagnostic("NXGO managed_core bootstrap failed: " + ex);
            throw;
        }
    }

    internal static void WriteDiagnostic(string message)
    {
        Console.WriteLine(message);
        var path = Environment.GetEnvironmentVariable("NXGO_AGENT_DIAGNOSTICS");
        if (string.IsNullOrWhiteSpace(path)) return;
        var line = DateTime.UtcNow.ToString("O") + " " + message + Environment.NewLine;
        File.AppendAllText(path, line);
    }
}
