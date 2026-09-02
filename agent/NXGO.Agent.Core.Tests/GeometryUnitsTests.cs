using NXGO.Agent.Core;
using Xunit;

namespace NXGO.Agent.Core.Tests;

public sealed class GeometryUnitsTests
{
    [Fact]
    public void Metric_mass_properties_convert_kg_m_to_mm_contract_exactly_once()
    {
        var contract = GeometryUnitContract.ForPartUnits(NxgoLengthUnit.Millimeter);
        Assert.Equal(UfMassAnalysisUnits.KilogramsMeters, contract.UfMassUnits);

        // 100 x 50 x 25 mm analytical block represented in UF kg/m units.
        var uf = new double[47];
        uf[0] = 0.0175;     // m^2
        uf[1] = 0.000125;   // m^3
        uf[2] = 0.975;      // kg (arbitrary material density result)
        uf[3] = 0.050;      // m
        uf[4] = 0.025;      // m
        uf[5] = 0.0125;     // m

        var got = contract.NormalizeMassProperties(uf);

        Assert.Equal(17_500.0, got.Area, 9);
        Assert.Equal(125_000.0, got.Volume, 9);
        Assert.Equal(0.975, got.Mass, 12);
        Assert.Equal(50.0, got.Centroid[0], 12);
        Assert.Equal(25.0, got.Centroid[1], 12);
        Assert.Equal(12.5, got.Centroid[2], 12);
        Assert.Equal("mm", got.LengthUnit);
        Assert.Equal("mm^2", got.AreaUnit);
        Assert.Equal("mm^3", got.VolumeUnit);
        Assert.Equal("kg", got.MassUnit);
    }

    [Fact]
    public void Imperial_mass_properties_are_already_in_public_part_units()
    {
        var contract = GeometryUnitContract.ForPartUnits(NxgoLengthUnit.Inch);
        Assert.Equal(UfMassAnalysisUnits.PoundsInches, contract.UfMassUnits);

        var uf = new double[47];
        uf[0] = 12.5;
        uf[1] = 3.25;
        uf[2] = 1.5;
        uf[3] = 2.0;
        uf[4] = 3.0;
        uf[5] = 4.0;

        var got = contract.NormalizeMassProperties(uf);
        Assert.Equal(12.5, got.Area);
        Assert.Equal(3.25, got.Volume);
        Assert.Equal(1.5, got.Mass);
        Assert.Equal(new[] { 2.0, 3.0, 4.0 }, got.Centroid);
        Assert.Equal("in", got.LengthUnit);
        Assert.Equal("lb", got.MassUnit);
    }

    [Fact]
    public void Bounding_box_is_identity_in_part_length_units()
    {
        var contract = GeometryUnitContract.ForPartUnits(NxgoLengthUnit.Millimeter);
        var got = contract.NormalizeBoundingBox(new[] { -10.0, 5.0, 2.0, 100.0, 50.0, 25.0 });

        Assert.Equal(new[] { -10.0, 5.0, 2.0 }, got.MinCorner);
        Assert.Equal(new[] { 100.0, 50.0, 25.0 }, got.MaxCorner);
        Assert.Equal(new[] { 110.0, 45.0, 23.0 }, got.Dimensions);
        Assert.Equal("mm", got.LengthUnit);
    }

    [Fact]
    public void Invalid_UF_arrays_are_rejected_fail_closed()
    {
        var contract = GeometryUnitContract.MillimeterKilogram;
        Assert.Throws<ArgumentException>(() => contract.NormalizeMassProperties(new double[5]));
        Assert.Throws<ArgumentException>(() => contract.NormalizeBoundingBox(new double[5]));
    }
}
