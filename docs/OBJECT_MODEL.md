# Object, lifetime and transaction model

## 1. Why handles are required

NXOpen objects are runtime objects tied to a specific NX process/session. They cannot be serialized into Go or treated as durable identifiers.

NXGO therefore uses opaque remote handles.

## 2. ObjectRef

Conceptual shape:

```go
type ObjectRef struct {
    SessionID string
    ObjectID  uint64
    Kind      ObjectKind
    ScopeID   uint64
}
```

Applications SHOULD use typed proxies (`*Part`, `*Face`, `*Feature`) rather than manipulating `ObjectRef` directly.

## 3. Identity rules

An object handle is valid only when:

- the same NX process/session is alive;
- the registry entry still exists;
- the owning lease/scope has not been released;
- the underlying NX object remains valid.

After worker restart every old handle is stale, even if a newly opened part has the same filename/native tag.

## 4. Scope hierarchy

Recommended scopes:

```text
Client/session scope
  |- transaction scope
  |- explicit object scope
  `- operation-temporary scope
```

Temporary builders and query objects are released at operation end. Durable proxies live in a caller/client scope until explicitly released or connection close.

## 5. Release

The protocol supports batch release of handles. Go finalizers MUST NOT be relied upon for correctness; they MAY be a leak-safety fallback only.

Preferred API:

```go
scope := client.NewScope()
defer scope.Close()

faces, err := scope.Part(part).Faces(ctx)
```

## 6. Registry limits

Agent enforces:

- max handles per client;
- max temporary handles per operation;
- optional idle lease expiry;
- leak metrics and warnings.

Limit exhaustion returns a typed resource error rather than destabilizing NX.

## 7. Collections

Bulk collections SHOULD be materialized as value summaries when the caller needs only metadata. Return remote handles only when subsequent object operations are expected.

## 8. Transactions

NXGO transaction is a logical unit mapped to NX undo facilities where possible.

Guarantees:

- commands within one transaction execute in order;
- failure triggers configured rollback attempt;
- result reports whether rollback succeeded;
- handles created then rolled back become invalid.

Non-guarantees:

- distributed ACID semantics;
- rollback of external side effects such as already-written files unless the workflow stages them atomically;
- rollback across NX process crash.

## 9. File-producing operations

Exports should write to a temporary/staging path then atomically promote where the filesystem permits. A failed transaction cleans staging artifacts when possible.

## 10. Dirty session policy

Session becomes dirty when NX state can no longer be trusted, for example failed rollback or serious native exception. Interactive mode reports and leaves recovery to policy/user. Worker mode defaults to recycle-before-next-mutating-job.