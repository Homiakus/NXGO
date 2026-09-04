using System;
using System.Collections.Generic;
using System.Globalization;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;

namespace NXGO.Agent.NXHost;

public static partial class EntryPoint
{
    private static Task<byte[]> StartCreateExpression(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var name = GetString(payload, "name", string.Empty).Trim();
        var formula = GetString(payload, "formula", string.Empty).Trim();
        var unitsStr = GetString(payload, "units", string.Empty).Trim();
        var desc = GetString(payload, "description", string.Empty).Trim();

        if (string.IsNullOrWhiteSpace(name))
            throw new ArgumentException("expression name cannot be empty");
        if (string.IsNullOrWhiteSpace(formula))
            throw new ArgumentException("expression formula cannot be empty");

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            string equation = formula.Contains("=") ? formula : $"{name} = {formula}";

            Unit unit = null!;
            if (!string.IsNullOrWhiteSpace(unitsStr))
            {
                unit = ResolveUnit(part, unitsStr);
            }

            Expression expr;
            if (unit != null)
            {
                expr = part.Expressions.CreateWithUnits(equation, unit);
            }
            else
            {
                expr = part.Expressions.Create(equation);
            }

            var exprHandle = Registry.Register(expr, "Expression", ownerObjectId: partHandle.ObjectId);

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["expression_ref"] = FormatHandle(exprHandle, expr),
                ["name"] = expr.Name,
                ["formula"] = expr.RightHandSide,
                ["value"] = GetExpressionNumericValue(expr),
                ["string_value"] = GetExpressionStringValue(expr),
                ["type"] = expr.Type ?? "Number",
                ["units"] = expr.Units != null ? expr.Units.Symbol : string.Empty,
                ["native_tag"] = expr.Tag,
            });
        }, token));
    }

    private static Task<byte[]> StartQueryExpressions(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var targetName = GetString(payload, "name", string.Empty).Trim();

        return MapRead(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");

            var items = new List<object>();

            if (!string.IsNullOrWhiteSpace(targetName))
            {
                Expression expr = null!;
                try
                {
                    expr = part.Expressions.FindObject(targetName);
                }
                catch
                {
                    // expression not found
                }

                if (expr != null)
                {
                    var exprHandle = Registry.Register(expr, "Expression", ownerObjectId: partHandle.ObjectId);
                    items.Add(new Dictionary<string, object>
                    {
                        ["expression_ref"] = FormatHandle(exprHandle, expr),
                        ["name"] = expr.Name,
                        ["formula"] = expr.RightHandSide,
                        ["value"] = GetExpressionNumericValue(expr),
                        ["string_value"] = GetExpressionStringValue(expr),
                        ["type"] = expr.Type ?? "Number",
                        ["units"] = expr.Units != null ? expr.Units.Symbol : string.Empty,
                        ["native_tag"] = expr.Tag,
                    });
                }
            }
            else
            {
                foreach (Expression expr in part.Expressions)
                {
                    var exprHandle = Registry.Register(expr, "Expression", ownerObjectId: partHandle.ObjectId);
                    items.Add(new Dictionary<string, object>
                    {
                        ["expression_ref"] = FormatHandle(exprHandle, expr),
                        ["name"] = expr.Name,
                        ["formula"] = expr.RightHandSide,
                        ["value"] = GetExpressionNumericValue(expr),
                        ["string_value"] = GetExpressionStringValue(expr),
                        ["type"] = expr.Type ?? "Number",
                        ["units"] = expr.Units != null ? expr.Units.Symbol : string.Empty,
                        ["native_tag"] = expr.Tag,
                    });
                }
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["expressions"] = items
            });
        }, token));
    }

    private static Task<byte[]> StartEditExpression(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var formula = GetString(payload, "formula", string.Empty).Trim();
        var targetName = GetString(payload, "name", string.Empty).Trim();

        ObjectHandleToken? exprHandle = null;
        if (payload.TryGetValue("expression_ref", out var eRaw) && eRaw is Dictionary<string, object> eDict && eDict.Count > 0)
        {
            exprHandle = RequireHandle(new Dictionary<string, object> { ["expr"] = eDict }, "expr", "Expression");
        }

        if (exprHandle == null && string.IsNullOrWhiteSpace(targetName))
        {
            throw new ArgumentException("either expression_ref or name must be provided to edit expression");
        }
        if (string.IsNullOrWhiteSpace(formula))
        {
            throw new ArgumentException("formula cannot be empty");
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            Expression expr;
            if (exprHandle != null)
            {
                expr = (Expression)Registry.Resolve(exprHandle, "Expression");
            }
            else
            {
                expr = part.Expressions.FindObject(targetName);
            }

            if (expr == null)
            {
                throw new InvalidOperationException("expression not found: " + targetName);
            }

            string rhs = formula;
            if (formula.Contains("="))
            {
                var idx = formula.IndexOf('=');
                rhs = formula.Substring(idx + 1).Trim();
            }

            expr.RightHandSide = rhs;

            var regHandle = Registry.Register(expr, "Expression", ownerObjectId: partHandle.ObjectId);

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["expression_ref"] = FormatHandle(regHandle, expr),
                ["name"] = expr.Name,
                ["formula"] = expr.RightHandSide,
                ["value"] = GetExpressionNumericValue(expr),
                ["string_value"] = GetExpressionStringValue(expr),
                ["updated"] = true,
            });
        }, token));
    }

    private static Task<byte[]> StartDeleteExpression(
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var partHandle = RequireHandle(payload, "part_ref", "Part");
        var targetName = GetString(payload, "name", string.Empty).Trim();

        ObjectHandleToken? exprHandle = null;
        if (payload.TryGetValue("expression_ref", out var eRaw) && eRaw is Dictionary<string, object> eDict && eDict.Count > 0)
        {
            exprHandle = RequireHandle(new Dictionary<string, object> { ["expr"] = eDict }, "expr", "Expression");
        }

        if (exprHandle == null && string.IsNullOrWhiteSpace(targetName))
        {
            throw new ArgumentException("either expression_ref or name must be provided to delete expression");
        }

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();
            var part = (Part)Registry.Resolve(partHandle, "Part");
            Journal.MarkStarted(requestId);

            Expression expr;
            if (exprHandle != null)
            {
                expr = (Expression)Registry.Resolve(exprHandle, "Expression");
            }
            else
            {
                expr = part.Expressions.FindObject(targetName);
            }

            if (expr == null)
            {
                throw new InvalidOperationException("expression not found: " + targetName);
            }

            part.Expressions.Delete(expr);

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["deleted"] = true
            });
        }, token));
    }

    private static Unit ResolveUnit(Part part, string unitStr)
    {
        try
        {
            return part.UnitCollection.FindObject(unitStr);
        }
        catch
        {
            switch (unitStr.Trim().ToLowerInvariant())
            {
                case "mm":
                case "millimeter":
                case "millimeters":
                    return part.UnitCollection.FindObject("MilliMeter");
                case "in":
                case "inch":
                case "inches":
                    return part.UnitCollection.FindObject("Inch");
                case "deg":
                case "degree":
                case "degrees":
                    return part.UnitCollection.FindObject("Degrees");
                case "rad":
                case "radian":
                case "radians":
                    return part.UnitCollection.FindObject("Radians");
                default:
                    return null!;
            }
        }
    }

    private static double GetExpressionNumericValue(Expression expr)
    {
        try
        {
            return expr.Value;
        }
        catch
        {
            return 0.0;
        }
    }

    private static string GetExpressionStringValue(Expression expr)
    {
        try
        {
            return expr.StringValue ?? string.Empty;
        }
        catch
        {
            return string.Empty;
        }
    }
}
