# Go API design specification

## 1. Goals

The API MUST feel idiomatic to Go developers and SHOULD hide NXOpen implementation patterns unless the caller explicitly opts into `raw`.

## 2. Package principles

- small stable root package;
- domain subpackages only when they reduce coupling;
- `context.Context` first parameter for blocking/remote work;
- `(T, error)` conventions;
- functional options only for genuinely optional cross-cutting configuration;
- typed request structs for operations expected to grow;
- no public dependency on protobuf-generated transport types;
- no public dependency on .NET/NXOpen naming.

## 3. Connection

Target shape:

```go
client, err := nxgo.Connect(ctx,
    nxgo.WithMode(nxgo.AttachOrStart),
    nxgo.WithVersion("2606"),
)
```

Alternative explicit APIs:

```go
client, err := nxgo.Attach(ctx, selector)
client, err := nxgo.Start(ctx, startOptions)
```

`Client` owns transport and negotiated session metadata. `Close` releases client scopes; it MUST NOT silently terminate a user-owned interactive NX process.

## 4. Domain services

```go
type Client struct {
    Parts       PartsService
    Assemblies  AssemblyService
    Validation  ValidationService
    Events      EventService
    Logs        LogService
    Raw         RawService
}
```

Services are interfaces only where mocking/substitution is useful. Avoid interface-for-everything style.

## 5. Domain values

Define NX-independent values:

```go
type Length float64
func MM(v float64) Length
func Inch(v float64) Length

type Point3 struct { X, Y, Z float64 }
type Vector3 struct { X, Y, Z float64 }
type Matrix3 struct { M [3][3]float64 }
```

Unit conversions occur at the boundary. Requests MUST identify unit expectations. Ambiguous unitless length inputs SHOULD be avoided.

## 6. Remote object proxies

```go
type Part struct {
    client *Client
    ref    ObjectRef
}
```

A proxy is not the NX object. It contains a session-scoped reference and convenience methods. Stale references return `ErrStaleObject` or `ErrSessionLost`.

## 7. Modeling example

```go
feature, err := part.Modeling.CreateHole(ctx, HoleRequest{
    Diameter: MM(6),
    Limit:    ThroughAll(),
    Position: Point2{X: 20, Y: 30},
})
```

The public API MUST NOT expose Builder lifecycle unless under the generated raw namespace.

## 8. Drawing example

```go
dwg, err := part.Drawings.Generate(ctx, DrawingRequest{
    Standard:     ESKD,
    Sheet:        AutoSheet(),
    Views:        AutoViews(),
    Dimensions:   FromPMI(),
    HoleCallouts: true,
    Centerlines:  true,
    Validate:     true,
})
```

The result includes warnings and a quality report rather than hiding non-fatal issues.

## 9. Transactions

```go
err := client.Transaction(ctx, "create bracket", func(tx *Tx) error {
    part, err := tx.Parts.New(ctx, NewPartRequest{...})
    if err != nil { return err }
    _, err = part.Modeling.Extrude(ctx, req)
    return err
})
```

If the closure returns an error, NXGO attempts rollback. Failure to rollback upgrades the returned error severity and marks the session potentially dirty.

## 10. Bulk APIs

Prefer:

```go
analysis, err := part.Analyze(ctx, AnalysisRequest{
    BoundingBox: true,
    Mass:        true,
    Holes:       true,
    Faces:       FaceSummary,
})
```

over chatty per-property calls.

## 11. Events and logs

```go
stream, err := client.Events.Subscribe(ctx, EventFilter{...})
for ev := range stream.Events() { ... }
```

Streams MUST surface terminal errors separately from ordinary data. Backpressure policy MUST be documented per stream.

## 12. Error model

Stable categories:

```go
var (
    ErrNotConnected       = errors.New("nxgo: not connected")
    ErrSessionLost        = errors.New("nxgo: session lost")
    ErrTimeout            = errors.New("nxgo: timeout")
    ErrUnsupported        = errors.New("nxgo: unsupported capability")
    ErrInvalidObject      = errors.New("nxgo: invalid object")
    ErrStaleObject        = errors.New("nxgo: stale object")
    ErrLicenseUnavailable = errors.New("nxgo: license unavailable")
)
```

Detailed error:

```go
type Error struct {
    Kind       Kind
    Operation  string
    NXCode     int
    Message    string
    Recoverable bool
    RunID      string
    TestID     string
}
```

`errors.Is/As` MUST work.

## 13. Raw API

Raw API is explicitly namespaced and may mirror NX concepts more closely:

```go
obj, err := client.Raw.Invoke(ctx, raw.Call{...})
```

Generated typed raw wrappers are preferred over stringly-typed invocation. Dynamic invocation exists as the final escape hatch and is disabled or restricted in hardened deployments.

## 14. Compatibility guarantees

- workflow/domain APIs: semantic versioning, strongest compatibility;
- generated raw API: release manifest specific, weaker source stability;
- dynamic raw API: runtime compatibility only;
- protocol DTOs: internal package, no application-level compatibility promise.

## 15. API review checklist

Every new public API must answer:

- Is it high-level enough to avoid repeated IPC?
- What NX state does it mutate?
- Is it idempotent?
- Can it roll back?
- What object lifetimes result?
- What units are used?
- Which capabilities/releases are required?
- What logs/events are emitted?
- How is it tested without NX and with real NX?