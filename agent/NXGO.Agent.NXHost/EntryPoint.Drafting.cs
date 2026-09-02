using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;
using NXOpen.Drawings;

namespace NXGO.Agent.NXHost;

public static partial class EntryPoint
{
    private const int MaxDraftingSnapshotSheets = 1024;

    private static Task<byte[]> StartDraftingCreateSheet(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var sheetName = GetString(payload, "sheet_name", string.Empty);
        if (string.IsNullOrWhiteSpace(sheetName)) sheetName = "Sheet_1";

        var height = GetDouble(payload, "height", 297.0);
        var length = GetDouble(payload, "length", 420.0);
        var numerator = GetDouble(payload, "scale_numerator", 1.0);
        var denominator = GetDouble(payload, "scale_denominator", 1.0);
        if (height <= 0.0 || length <= 0.0)
        {
            throw new ArgumentException("drawing sheet height and length must be positive");
        }
        if (numerator <= 0.0 || denominator <= 0.0)
        {
            throw new ArgumentException("drawing sheet scale numerator and denominator must be positive");
        }

        var unitsText = GetString(payload, "units", "mm").Trim().ToLowerInvariant();
        DrawingSheet.Unit sheetUnit;
        switch (unitsText)
        {
            case "":
            case "mm":
            case "millimeter":
            case "millimeters":
                sheetUnit = DrawingSheet.Unit.Millimeters;
                break;
            case "in":
            case "inch":
            case "inches":
                sheetUnit = DrawingSheet.Unit.Inches;
                break;
            default:
                throw new ArgumentException("unsupported drawing sheet units: " + unitsText);
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            if (Registry.Count >= Registry.Capacity)
            {
                throw new HandleRegistryCapacityException(Registry.Capacity);
            }
            Journal.MarkStarted(requestId);

            var sheet = part.DrawingSheets.InsertSheet(
                sheetName,
                sheetUnit,
                height,
                length,
                numerator,
                denominator,
                DrawingSheet.ProjectionAngleType.FirstAngle);

            var handle = Registry.Register(
                sheet,
                "DrawingSheet",
                ownerObjectId: partHandle.ObjectId);
            uint nativeTag = 0;
            try { nativeTag = (uint)sheet.Tag; } catch { }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["sheet_ref"] = FormatHandle(handle, sheet),
                ["sheet_name"] = sheet.Name ?? sheetName,
                ["height"] = sheet.Height,
                ["length"] = sheet.Length,
                ["native_tag"] = nativeTag,
            });
        }, token));
    }

    private static Task<byte[]> StartDraftingQuerySheets(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            var sheets = new List<DrawingSheet>();
            foreach (DrawingSheet sheet in part.DrawingSheets)
            {
                sheets.Add(sheet);
                if (sheets.Count > MaxDraftingSnapshotSheets)
                {
                    throw new ArgumentException("drawing sheet snapshot exceeds canonical safety limit");
                }
            }

            var result = new List<object>(sheets.Count);
            foreach (var sheet in sheets)
            {
                double numerator = 1.0;
                double denominator = 1.0;
                try { sheet.GetScale(out numerator, out denominator); } catch { }
                uint nativeTag = 0;
                try { nativeTag = (uint)sheet.Tag; } catch { }

                // Query results are value snapshots. No registry slot is
                // allocated for a read-only listing, avoiding repeated-query
                // handle leaks present in the legacy Agent.
                result.Add(new Dictionary<string, object>
                {
                    ["sheet_ref"] = new Dictionary<string, object>(),
                    ["name"] = sheet.Name ?? string.Empty,
                    ["height"] = sheet.Height,
                    ["length"] = sheet.Length,
                    ["numerator"] = numerator,
                    ["denominator"] = denominator,
                    ["native_tag"] = nativeTag,
                });
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["sheets"] = result,
            });
        }, token));
    }

    private static Task<byte[]> StartDraftingExportPdf(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var outputPath = GetString(payload, "output_pdf_path", string.Empty);
        if (string.IsNullOrWhiteSpace(outputPath))
        {
            throw new ArgumentException("output_pdf_path is required");
        }
        if (!string.Equals(Path.GetExtension(outputPath), ".pdf", StringComparison.OrdinalIgnoreCase))
        {
            throw new ArgumentException("output_pdf_path must end with .pdf");
        }

        var colorMode = GetString(payload, "color_mode", string.Empty).Trim().ToLowerInvariant();
        if (colorMode.Length > 0 && colorMode != "black_and_white")
        {
            // Legacy silently ignored color/grayscale. Until those NX enum
            // semantics are verified on the pinned release, fail closed.
            throw new ArgumentException("canonical PDF export currently supports only black_and_white color mode");
        }
        var requestedSheets = GetUniqueStringArray(payload, "sheet_names");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");

            var outputDirectory = Path.GetDirectoryName(outputPath);
            if (!string.IsNullOrWhiteSpace(outputDirectory) && !Directory.Exists(outputDirectory))
            {
                throw new ArgumentException("PDF output directory does not exist: " + outputDirectory);
            }

            var selected = SelectDrawingSheets(part, requestedSheets);
            if (selected.Count == 0)
            {
                throw new ArgumentException("PDF export requires at least one drawing sheet");
            }

            Journal.MarkStarted(requestId);
            using (var scope = new BuilderScope<PrintPDFBuilder>(
                part.PlotManager.CreatePrintPdfbuilder(),
                builder => { try { builder.Destroy(); } catch { } }))
            {
                var builder = scope.Builder;
                builder.Action = PrintPDFBuilder.ActionOption.Native;
                builder.Filename = outputPath;
                builder.Colors = PrintPDFBuilder.Color.BlackOnWhite;
                builder.SourceBuilder.SetSheets(selected.ToArray());
                scope.CommitOnce(b => b.Commit());
            }

            if (!File.Exists(outputPath))
            {
                throw new IOException("NX reported PDF export completion but output file does not exist: " + outputPath);
            }
            var size = new FileInfo(outputPath).Length;
            if (size <= 0)
            {
                throw new IOException("NX produced an empty PDF artifact: " + outputPath);
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["exported_path"] = outputPath.Replace('\\', '/'),
                ["file_size_bytes"] = size,
            });
        }, token));
    }

    private static List<NXObject> SelectDrawingSheets(Part part, HashSet<string> requestedNames)
    {
        var selected = new List<NXObject>();
        var missing = new HashSet<string>(requestedNames, StringComparer.OrdinalIgnoreCase);
        foreach (DrawingSheet sheet in part.DrawingSheets)
        {
            if (requestedNames.Count == 0 || requestedNames.Contains(sheet.Name ?? string.Empty))
            {
                selected.Add(sheet);
                missing.Remove(sheet.Name ?? string.Empty);
            }
        }
        if (missing.Count > 0)
        {
            var names = new List<string>(missing);
            names.Sort(StringComparer.OrdinalIgnoreCase);
            throw new ArgumentException("requested drawing sheets were not found: " + string.Join(", ", names));
        }
        return selected;
    }

    private static HashSet<string> GetUniqueStringArray(Dictionary<string, object> source, string key)
    {
        var result = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var item in GetArray(source, key))
        {
            var value = Convert.ToString(item, CultureInfo.InvariantCulture) ?? string.Empty;
            value = value.Trim();
            if (value.Length == 0)
            {
                throw new ArgumentException(key + " cannot contain an empty name");
            }
            if (!result.Add(value))
            {
                throw new ArgumentException(key + " contains duplicate sheet name: " + value);
            }
        }
        return result;
    }
}
