package nx_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func TestRealNXWorkerSecurityHandshakeBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	// 1. Legitimate supervisor session works seamlessly with bound nonce.
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("legitimate supervisor ping failed: %v", err)
	}

	tempDir := t.TempDir()
	origPath := filepath.Join(tempDir, "security_test_orig.prt")
	saveAsPath := filepath.Join(tempDir, "security_test_savedas.prt")

	part, err := session.NewPart(ctx, origPath, "mm")
	if err != nil {
		t.Fatalf("failed to create new part: %v", err)
	}

	// Add block to mutate part.
	_, err = part.CreateBlock(ctx, nxgo.BlockParams{
		Length: 50.0,
		Width:  50.0,
		Height: 50.0,
	})
	if err != nil {
		t.Fatalf("failed to create block: %v", err)
	}

	// 2. Test SaveAs & ForceCloseDiscard.
	saveAsResp, err := part.SaveAs(ctx, saveAsPath)
	if err != nil {
		t.Fatalf("part.SaveAs failed: %v", err)
	}
	if !saveAsResp.Saved {
		t.Fatalf("expected part to be saved via SaveAs, got %+v", saveAsResp)
	}
	info, err := os.Stat(saveAsPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected saved-as file to exist with size > 0: err=%v, stat=%+v", err, info)
	}

	if err := part.ForceCloseDiscard(ctx); err != nil {
		t.Fatalf("part.ForceCloseDiscard failed: %v", err)
	}
	t.Log("SaveAs and ForceCloseDiscard passed cleanly")

	// 3. Test OS Single-Instance Mutual Exclusion:
	// While legitimate supervisor holds the pipe open, any concurrent dial MUST be rejected with ERROR_PIPE_BUSY.
	pipePath := fmt.Sprintf(`\\.\pipe\%s`, worker.Config.PipeName)
	concurrentDialConn, err := pipe.DialPipe(ctx, pipePath)
	if err == nil {
		_ = concurrentDialConn.Close()
		t.Fatal("expected concurrent dial to fail due to single-instance pipe exclusivity, but it succeeded")
	}
	t.Logf("concurrent pipe dial properly rejected by OS: %v", err)
	if !strings.Contains(err.Error(), "busy") {
		t.Fatalf("expected pipe busy error, got: %v", err)
	}

	// 4. Now close supervisor client to allow subsequent sequential connection tests.
	_ = worker.Client.Close()
	time.Sleep(200 * time.Millisecond)

	// 5. Test Unauthorized connection: direct RPC without handshake rejected.
	rogueConn1, err := pipe.DialPipe(ctx, pipePath)
	if err != nil {
		t.Fatalf("failed to dial worker pipe after supervisor disconnect: %v", err)
	}
	rogueClient1 := pipe.NewClient(rogueConn1)

	unauthResp, err := rogueClient1.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-unauthorized-probe",
		Op:        "nx.ping",
	})
	if err == nil && (unauthResp == nil || unauthResp.Status == protocol.StatusOK) {
		_ = rogueClient1.Close()
		t.Fatalf("expected unauthenticated call to be rejected, got status=%v, resp=%+v", unauthResp.Status, unauthResp)
	}
	if unauthResp != nil && unauthResp.Error != nil {
		t.Logf("unauthenticated probe properly rejected: category=%s, msg=%s", unauthResp.Error.Category, unauthResp.Error.Message)
		if unauthResp.Error.Category != "UNAUTHORIZED" {
			_ = rogueClient1.Close()
			t.Fatalf("expected UNAUTHORIZED category, got %s", unauthResp.Error.Category)
		}
	}
	_ = rogueClient1.Close()
	time.Sleep(200 * time.Millisecond)

	// 6. Test Bogus Nonce Handshake: connection rejected.
	rogueConn2, err := pipe.DialPipe(ctx, pipePath)
	if err != nil {
		t.Fatalf("failed to dial worker pipe for bogus handshake test: %v", err)
	}
	rogueClient2 := pipe.NewClient(rogueConn2)

	_, err = rogueClient2.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		SDKVersion:      "v0.1.0",
		ClientPID:       os.Getpid(),
		Nonce:           "bogus-unauthorized-nonce-xyz",
	})
	if err == nil {
		_ = rogueClient2.Close()
		t.Fatal("expected handshake with bogus nonce to fail, but it succeeded")
	}
	t.Logf("bogus nonce handshake properly rejected with: %v", err)
	if !strings.Contains(err.Error(), "UNAUTHORIZED") && !strings.Contains(err.Error(), "handshake nonce mismatch") {
		_ = rogueClient2.Close()
		t.Fatalf("expected UNAUTHORIZED or nonce mismatch error, got: %v", err)
	}
	_ = rogueClient2.Close()
	time.Sleep(200 * time.Millisecond)

	// 7. Legitimate Re-authentication with valid Nonce & Hardened Allowlist Check.
	legitReconn, err := pipe.DialPipe(ctx, pipePath)
	if err != nil {
		t.Fatalf("failed to dial worker pipe for re-authentication: %v", err)
	}
	legitClient := pipe.NewClient(legitReconn)
	defer legitClient.Close()

	hsResp, err := legitClient.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		SDKVersion:      "v0.1.0",
		ClientPID:       os.Getpid(),
		Nonce:           worker.Config.WorkerNonce,
	})
	if err != nil {
		t.Fatalf("re-authentication with valid nonce failed: %v", err)
	}
	t.Logf("re-authentication succeeded with sessionID=%s", hsResp.SessionID)

	// Verify ping works after legitimate re-authentication.
	reconnSession := nxgo.WrapClient(legitClient, hsResp.SessionID, hsResp.Epoch, hsResp.NXRelease)
	if err := reconnSession.Ping(ctx); err != nil {
		t.Fatalf("ping after re-authentication failed: %v", err)
	}
	t.Log("ping succeeded after legitimate re-authentication")

	// Verify unsupported operation outside allowlist is rejected.
	unsupportedResp, err := legitClient.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-unsupported-op",
		Op:        "raw.journal_eval",
	})
	if err == nil && (unsupportedResp == nil || unsupportedResp.Status == protocol.StatusOK) {
		t.Fatalf("expected unsupported op to be rejected, got status=%v, resp=%+v", unsupportedResp.Status, unsupportedResp)
	}
	if unsupportedResp != nil && unsupportedResp.Error != nil {
		t.Logf("unsupported op rejected: category=%s, msg=%s", unsupportedResp.Error.Category, unsupportedResp.Error.Message)
		if unsupportedResp.Error.Category != "UNSUPPORTED_OPERATION" {
			t.Fatalf("expected UNSUPPORTED_OPERATION category, got %s", unsupportedResp.Error.Category)
		}
	}
	t.Log("operation allowlist enforcement passed")
}
