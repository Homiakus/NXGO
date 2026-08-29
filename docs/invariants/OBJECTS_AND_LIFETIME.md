# Object and lifetime invariants

## NXGO-INV-OBJ-001 — dead NX objects are never trusted

**MUST NOT:** assume an object remains alive because the Go proxy still exists.

NX topology/features/parts can become invalid after delete, undo, part close/reopen, topology replacement, update or process restart.

**MUST:** validate handle ownership/liveness at the Agent boundary and map failures to `ErrStaleObject`/typed NX errors.

## NXGO-INV-OBJ-002 — handles carry session identity and epoch

**MUST NOT:** identify a remote object only by an incrementing integer or NX tag.

**MUST:** bind references to at least session identity + process/session epoch + object ID; use generation when an object slot can be recycled.

Example conceptual shape:

```go
type ObjectRef struct {
    SessionID  string
    Epoch      uint64
    ObjectID   uint64
    Generation uint32
    TypeID     uint32
}
```

After NX restart, every handle from the previous epoch is invalid even if IDs/tags are reused.

## NXGO-INV-OBJ-003 — live Siemens objects stop at Agent boundary

**MUST NOT:** serialize `NXObject`, `TaggedObject`, `NXRemotableObject`, CLR remoting proxies, raw pointers or Siemens implementation types into the Go protocol.

**MUST:** convert values to stable NXGO DTOs or scoped handles.

**Why:** Siemens object identity/lifetime is tied to the NX process/runtime and is not a stable cross-language protocol contract.

## NXGO-INV-OBJ-004 — remote handles are bounded resources

**MUST NOT:** let clients allocate unbounded object handles for a long-running session.

**MUST:** support explicit scopes/releases, registry quotas and automatic cleanup on disconnect/session end.

**Tests:** enumerate large assemblies repeatedly and prove registry size returns to baseline after scope release; crash/disconnect must free client-owned handles.