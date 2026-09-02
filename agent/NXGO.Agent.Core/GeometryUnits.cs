namespace NXGO.Agent.Core;

public enum NxgoLengthUnit
{
    Millimeter = 0,
    Inch = 1,
}

/// <summary>
/// UF_MODL_ask_mass_props_3d unit selector values documented by NX Open.
/// </summary>
public enum UfMassAnalysisUnits
{
    PoundsInches = 1,
    PoundsFeet = 2,
    GramsCentimeters = 3,
    KilogramsMeters = 4,
}

public sealed class GeometryUnitContract
{
    private GeometryUnitContract(
        NxgoLengthUnit partLengthUnit,
        UfMassAnalysisUnits ufMassUnits,
        double lengthScale,
        double areaScale,
        double volumeScale,
        double massScale,
        string lengthSymbol,
        string areaSymbol,
        string volumeSymbol,
        string massSymbol)
    {
        PartLengthUnit = partLengthUnit;
        UfMassUnits = ufMassUnits;
        LengthScale = lengthScale;
        AreaScale = areaScale;
        VolumeScale = volumeScale;
        MassScale = massScale;
        LengthSymbol = lengthSymbol;
        AreaSymbol = areaSymbol;
        VolumeSymbol = volumeSymbol;
        MassSymbol = massSymbol;
    }

    public NxgoLengthUnit PartLengthUnit { get; }
    public UfMassAnalysisUnits UfMassUnits { get; }
    public double LengthScale { get; }
    public double AreaScale { get; }
    public double VolumeScale { get; }
    public double MassScale { get; }
    public string LengthSymbol { get; }
    public string AreaSymbol { get; }
    public string VolumeSymbol { get; }
    public string MassSymbol { get; }

    /// <summary>
    /// Metric public geometry is expressed in NX part units (mm, mm², mm³)
    /// and kg for mass. UF analysis mode 4 returns kg/m-based quantities, so
    /// conversion is performed exactly once here.
    /// </summary>
    public static GeometryUnitContract MillimeterKilogram { get; } = new GeometryUnitContract(
        NxgoLengthUnit.Millimeter,
        UfMassAnalysisUnits.KilogramsMeters,
        lengthScale: 1_000.0,
        areaScale: 1_000_000.0,
        volumeScale: 1_000_000_000.0,
        massScale: 1.0,
        lengthSymbol: "mm",
        areaSymbol: "mm^2",
        volumeSymbol: "mm^3",
        massSymbol: "kg");

    /// <summary>
    /// Imperial NX part geometry is expressed directly in inches and pounds
    /// using UF analysis mode 1, so no geometric scale conversion is needed.
    /// </summary>
    public static GeometryUnitContract InchPound { get; } = new GeometryUnitContract(
        NxgoLengthUnit.Inch,
        UfMassAnalysisUnits.PoundsInches,
        lengthScale: 1.0,
        areaScale: 1.0,
        volumeScale: 1.0,
        massScale: 1.0,
        lengthSymbol: "in",
        areaSymbol: "in^2",
        volumeSymbol: "in^3",
        massSymbol: "lb");

    public static GeometryUnitContract ForPartUnits(NxgoLengthUnit units)
    {
        switch (units)
        {
            case NxgoLengthUnit.Millimeter:
                return MillimeterKilogram;
            case NxgoLengthUnit.Inch:
                return InchPound;
            default:
                throw new ArgumentOutOfRangeException(nameof(units), units, "unsupported NX part units");
        }
    }

    public NormalizedMassProperties NormalizeMassProperties(double[] ufMassProps)
    {
        if (ufMassProps is null) throw new ArgumentNullException(nameof(ufMassProps));
        if (ufMassProps.Length < 6)
        {
            throw new ArgumentException("UF mass_props must contain at least six values", nameof(ufMassProps));
        }

        return new NormalizedMassProperties(
            area: ufMassProps[0] * AreaScale,
            volume: ufMassProps[1] * VolumeScale,
            mass: ufMassProps[2] * MassScale,
            centroid: new[]
            {
                ufMassProps[3] * LengthScale,
                ufMassProps[4] * LengthScale,
                ufMassProps[5] * LengthScale,
            },
            lengthUnit: LengthSymbol,
            areaUnit: AreaSymbol,
            volumeUnit: VolumeSymbol,
            massUnit: MassSymbol);
    }

    /// <summary>
    /// UF_MODL_ask_bounding_box returns coordinates in the owning part's
    /// length units. Therefore bounding-box values are already millimeters for
    /// a metric part and inches for an imperial part; no /1000 conversion is
    /// valid here.
    /// </summary>
    public NormalizedBoundingBox NormalizeBoundingBox(double[] minMax)
    {
        if (minMax is null) throw new ArgumentNullException(nameof(minMax));
        if (minMax.Length < 6)
        {
            throw new ArgumentException("UF bounding box must contain six values", nameof(minMax));
        }

        var min = new[] { minMax[0], minMax[1], minMax[2] };
        var max = new[] { minMax[3], minMax[4], minMax[5] };
        return new NormalizedBoundingBox(
            min,
            max,
            new[] { max[0] - min[0], max[1] - min[1], max[2] - min[2] },
            LengthSymbol);
    }
}

public sealed class NormalizedMassProperties
{
    public NormalizedMassProperties(
        double area,
        double volume,
        double mass,
        double[] centroid,
        string lengthUnit,
        string areaUnit,
        string volumeUnit,
        string massUnit)
    {
        Area = area;
        Volume = volume;
        Mass = mass;
        Centroid = centroid ?? throw new ArgumentNullException(nameof(centroid));
        LengthUnit = lengthUnit;
        AreaUnit = areaUnit;
        VolumeUnit = volumeUnit;
        MassUnit = massUnit;
    }

    public double Area { get; }
    public double Volume { get; }
    public double Mass { get; }
    public double[] Centroid { get; }
    public string LengthUnit { get; }
    public string AreaUnit { get; }
    public string VolumeUnit { get; }
    public string MassUnit { get; }
}

public sealed class NormalizedBoundingBox
{
    public NormalizedBoundingBox(double[] minCorner, double[] maxCorner, double[] dimensions, string lengthUnit)
    {
        MinCorner = minCorner ?? throw new ArgumentNullException(nameof(minCorner));
        MaxCorner = maxCorner ?? throw new ArgumentNullException(nameof(maxCorner));
        Dimensions = dimensions ?? throw new ArgumentNullException(nameof(dimensions));
        LengthUnit = lengthUnit;
    }

    public double[] MinCorner { get; }
    public double[] MaxCorner { get; }
    public double[] Dimensions { get; }
    public string LengthUnit { get; }
}
