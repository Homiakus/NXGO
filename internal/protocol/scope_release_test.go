package protocol

import "testing"

func TestObjectReleaseRequestSupportsScopeWithoutHandles(t *testing.T) {
	original := ObjectReleaseRequest{LeaseScopeID: "request-scope-42"}
	payload, err := EncodePayload(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePayload[ObjectReleaseRequest](payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.LeaseScopeID != original.LeaseScopeID {
		t.Fatalf("scope id changed across wire round-trip: got %q", decoded.LeaseScopeID)
	}
	if len(decoded.Handles) != 0 {
		t.Fatalf("scope release must not carry handles: got %d", len(decoded.Handles))
	}
}
