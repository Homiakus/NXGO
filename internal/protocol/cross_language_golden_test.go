package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestCrossLanguageGoldenHandshakeV2(t *testing.T) {
	golden := readProtocolGolden(t, "handshake_request_v2.json")
	var msg HandshakeRequest
	if err := json.Unmarshal(golden, &msg); err != nil {
		t.Fatalf("decode handshake golden: %v", err)
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("validate handshake golden: %v", err)
	}
	if msg.ProtocolVersion != (Version{Major: 2, Minor: 0}) {
		t.Fatalf("unexpected version: %+v", msg.ProtocolVersion)
	}
	if msg.RequestedMode != "dedicated-worker" || len(msg.RequestedFeatures) != 3 {
		t.Fatalf("extended handshake fields lost: mode=%q features=%v", msg.RequestedMode, msg.RequestedFeatures)
	}
	if msg.ClientUser != "инженер-日本語" {
		t.Fatalf("unicode client_user changed: %q", msg.ClientUser)
	}
	assertSemanticJSONRoundTrip(t, golden, msg)
}

func TestCrossLanguageGoldenExtendedRequestV2(t *testing.T) {
	golden := readProtocolGolden(t, "request_extended_v2.json")
	var req RequestEnvelope
	if err := json.Unmarshal(golden, &req); err != nil {
		t.Fatalf("decode request golden: %v", err)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("validate request golden: %v", err)
	}
	if req.CorrelationID != "corr-42" || req.RunID != "run-crosslang" || req.TestID != "test-wire-v2" {
		t.Fatalf("extended request identity fields lost: %+v", req)
	}
	if req.TimeoutMs != 15000 || req.TxID != "tx-optional-context" {
		t.Fatalf("timeout/tx fields lost: timeout=%d tx=%q", req.TimeoutMs, req.TxID)
	}
	if req.TraceMeta["suite"] != "cross-language" || req.TraceMeta["source"] != "shared-golden" {
		t.Fatalf("trace_meta changed: %+v", req.TraceMeta)
	}

	var payload map[string]any
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["path"] != `C:\NXGO\тест\部品 {draft} "quoted".prt` {
		t.Fatalf("path escaping changed: %q", payload["path"])
	}
	attack, ok := payload["attack"].(map[string]any)
	if !ok {
		t.Fatalf("attack field no longer a plain JSON object: %T", payload["attack"])
	}
	if attack["$type"] != "System.IO.FileInfo, System.IO.FileSystem" || attack["value"] != "must-remain-data" {
		t.Fatalf("$type payload metadata was not preserved as data: %+v", attack)
	}
	assertSemanticJSONRoundTrip(t, golden, req)
}

func TestCrossLanguageGoldenErrorResponseV2(t *testing.T) {
	golden := readProtocolGolden(t, "response_error_v2.json")
	var resp ResponseEnvelope
	if err := json.Unmarshal(golden, &resp); err != nil {
		t.Fatalf("decode error response golden: %v", err)
	}
	if resp.Status != StatusError || resp.Error == nil {
		t.Fatalf("unexpected error response: status=%s error=%+v", resp.Status, resp.Error)
	}
	if err := resp.Error.Validate(); err != nil {
		t.Fatalf("validate error response: %v", err)
	}
	if resp.Error.Op != "part.open" || resp.Error.CorrelationID != "corr-42" {
		t.Fatalf("extended error fields lost: %+v", resp.Error)
	}
	if len(resp.Warnings) != 2 || resp.Timing.TotalDuration != 6 {
		t.Fatalf("warnings/timing changed: warnings=%v timing=%+v", resp.Warnings, resp.Timing)
	}
	assertSemanticJSONRoundTrip(t, golden, resp)
}

func TestCrossLanguageGoldenProducedHandlesV2(t *testing.T) {
	golden := readProtocolGolden(t, "response_handles_v2.json")
	var resp ResponseEnvelope
	if err := json.Unmarshal(golden, &resp); err != nil {
		t.Fatalf("decode handle response golden: %v", err)
	}
	if resp.Status != StatusOK || len(resp.ProducedHandles) != 2 {
		t.Fatalf("unexpected handle response: status=%s handles=%d", resp.Status, len(resp.ProducedHandles))
	}
	feature := resp.ProducedHandles[0]
	body := resp.ProducedHandles[1]
	if feature.Generation != 3 || feature.Kind != "Feature" || feature.LeaseScopeID != "req-handles-001" {
		t.Fatalf("feature handle changed: %+v", feature)
	}
	if body.Generation != 5 || body.Kind != "Body" || body.NativeTag != 102 {
		t.Fatalf("body handle changed: %+v", body)
	}
	assertSemanticJSONRoundTrip(t, golden, resp)
}

func readProtocolGolden(t *testing.T, name string) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve protocol test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	path := filepath.Join(root, "testdata", "protocol", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read protocol golden %s: %v", name, err)
	}
	return b
}

func assertSemanticJSONRoundTrip(t *testing.T, golden []byte, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal protocol value: %v", err)
	}
	var want any
	var got any
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatalf("decode expected semantic JSON: %v", err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode actual semantic JSON: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("semantic JSON roundtrip mismatch\nwant: %s\n got: %s", string(golden), string(encoded))
	}
}
