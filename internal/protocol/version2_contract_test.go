package protocol

import (
    "encoding/json"
    "testing"
)

func TestProtocolV2RequiresGenerationAwareObjectHandles(t *testing.T) {
    if CurrentProtocolMajor != 2 {
        t.Fatalf("generation-aware handles require protocol major 2, got %d", CurrentProtocolMajor)
    }

    original := ObjectHandleWire{
        SessionID: "sess-v2",
        Epoch: 3,
        ObjectID: "obj-7",
        Generation: 11,
        Kind: "Body",
    }
    data, err := json.Marshal(original)
    if err != nil { t.Fatal(err) }
    var decoded ObjectHandleWire
    if err := json.Unmarshal(data, &decoded); err != nil { t.Fatal(err) }
    if decoded.Generation != original.Generation {
        t.Fatalf("generation lost across wire roundtrip: got %d want %d", decoded.Generation, original.Generation)
    }
}

func TestProtocolV1IsRejectedByCurrentV2Negotiation(t *testing.T) {
    current := Version{Major: CurrentProtocolMajor, Minor: CurrentProtocolMinor}
    if err := NegotiateVersion(Version{Major: 1, Minor: 0}, current); err == nil {
        t.Fatal("expected v1 client to be rejected by v2 server contract")
    }
}
