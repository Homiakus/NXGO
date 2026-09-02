package agentbundle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func loadAgentWorker(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "agent", "bundle", "AgentWorker.cs"))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read production AgentWorker.cs: %v", err)
	}
	return string(b)
}

func TestProductionAgentHardeningMarkers(t *testing.T) {
	src := loadAgentWorker(t)

	required := []string{
		"public sealed class OutcomeUnknownException",
		"TryCancelBeforeStart",
		"implicit work/display fallback is forbidden",
		"implicit first-body fallback is forbidden",
		"ResolveRegisteredHandle<T>",
		"RequireCreateOnlyFeatureOptions",
		"int units = imperial ? 1 : 4",
		"UF_MODL_ask_bounding_box already returns owning-part length units",
		"part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);",
	}
	for _, marker := range required {
		if !strings.Contains(src, marker) {
			t.Errorf("production Agent hardening marker missing: %q", marker)
		}
	}

	forbidden := []string{
		"if (session.Parts.Work != null) return session.Parts.Work;",
		"if (session.Parts.Display != null) return session.Parts.Display;",
		"foreach (Body b in part.Bodies) return b;",
		"try { part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False); } catch {}",
		"minMax[0] / 1000.0",
		"int units = 3; // 3 = Standard metric units in UF_MODL",
	}
	for _, marker := range forbidden {
		if strings.Contains(src, marker) {
			t.Errorf("production Agent contains forbidden pre-hardening construct: %q", marker)
		}
	}
}

func TestProductionAgentStrictHandleIdentity(t *testing.T) {
	src := loadAgentWorker(t)
	for _, marker := range []string{
		"reference is missing object_id",
		"reference is missing session_id",
		"reference is missing epoch",
		"reference is missing kind",
		"wrong object kind",
		"object kind/type mismatch",
	} {
		if !strings.Contains(src, marker) {
			t.Errorf("strict handle identity guard missing: %q", marker)
		}
	}
}

func TestProductionAgentMutationJournalContract(t *testing.T) {
	src := loadAgentWorker(t)
	required := []string{
		"public sealed class MutationJournal",
		"new MutationJournal(4096)",
		"SHA256.Create()",
		"ReturnCommitted",
		"Journal.MarkStarted(reqId)",
		"Journal.MarkCommitted(reqId, executionResult)",
		"Journal.MarkFailedBeforeStart",
		"Journal.MarkOutcomeUnknown",
		"previous execution outcome is unknown; request must not be replayed",
		"CANCELLED_BEFORE_START",
		"request_id reused with different operation or payload",
	}
	for _, marker := range required {
		if !strings.Contains(src, marker) {
			t.Errorf("production mutation-journal marker missing: %q", marker)
		}
	}
}
