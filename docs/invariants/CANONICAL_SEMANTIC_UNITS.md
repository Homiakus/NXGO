# Canonical semantic units

This is the repository contract for public geometry quantities. Wire payloads
carry an explicit `units` value where the operation has a unit-sensitive
length, and the SDK converts at the API boundary. Values in geometry payloads
are not unitless.

| Quantity | Canonical semantic unit | Applies to |
|---|---|---|
| `Point3D`, `Vector3D`, origin, position, direction, centroid | part length unit (`mm` or `inch`) | geometry, assembly, transforms |
| `length`, `width`, `height`, `diameter` | part length unit (`mm` or `inch`) | block, cylinder, drafting |
| `min_corner`, `max_corner`, `dimensions` | part length unit (`mm` or `inch`) | bounding boxes |
| `area` | square of part length unit (`mm²` or `in²`) | mass properties |
| `volume` | cube of part length unit (`mm³` or `in³`) | mass properties |
| `mass` | kilograms | mass properties |
| `angle`, when introduced by a public operation | degrees | angular parameters |
| `scale_numerator`, `scale_denominator` | dimensionless ratio | drafting |

`mm` is the default only for operations whose contract explicitly documents
that default. New public geometry operations must name their unit behavior in
the request/response contract and must not silently mix part units with UF
internal units. Conversion to NX/UF units belongs in the NX adapter boundary.
