package nx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/Homiakus/NXGO/pkg/nxgo"
)

type ProcessMetrics struct {
	WorkingSetBytes     uint64 `json:"working_set_bytes"`
	PeakWorkingSetBytes uint64 `json:"peak_working_set_bytes"`
	HandleCount         uint32 `json:"handle_count"`
}

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var (
	modPsapi                  = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo  = modPsapi.NewProc("GetProcessMemoryInfo")
	modKernel32               = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess           = modKernel32.NewProc("OpenProcess")
	procCloseHandle           = modKernel32.NewProc("CloseHandle")
	procGetProcessHandleCount = modKernel32.NewProc("GetProcessHandleCount")
)

const (
	processQueryInformation = 0x0400
	processVMRead           = 0x0010
)

func queryProcessMetrics(pid int) (ProcessMetrics, error) {
	var metrics ProcessMetrics
	if pid <= 0 {
		return metrics, nil
	}
	hProcess, _, _ := procOpenProcess.Call(
		uintptr(processQueryInformation|processVMRead),
		0,
		uintptr(pid),
	)
	if hProcess == 0 {
		return metrics, syscall.GetLastError()
	}
	defer procCloseHandle.Call(hProcess)

	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))
	r1, _, _ := procGetProcessMemoryInfo.Call(
		hProcess,
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.CB),
	)
	if r1 != 0 {
		metrics.WorkingSetBytes = uint64(pmc.WorkingSetSize)
		metrics.PeakWorkingSetBytes = uint64(pmc.PeakWorkingSetSize)
	}

	var handles uint32
	r2, _, _ := procGetProcessHandleCount.Call(
		hProcess,
		uintptr(unsafe.Pointer(&handles)),
	)
	if r2 != 0 {
		metrics.HandleCount = handles
	}

	return metrics, nil
}

type LatencyStats struct {
	MinMs float64 `json:"min_ms"`
	MaxMs float64 `json:"max_ms"`
	AvgMs float64 `json:"avg_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

func computeStats(durations []time.Duration) LatencyStats {
	if len(durations) == 0 {
		return LatencyStats{}
	}
	msList := make([]float64, len(durations))
	var sum float64
	for i, d := range durations {
		ms := float64(d.Microseconds()) / 1000.0
		msList[i] = ms
		sum += ms
	}
	sort.Float64s(msList)
	p50Idx := int(float64(len(msList)-1) * 0.50)
	p95Idx := int(float64(len(msList)-1) * 0.95)
	p99Idx := int(float64(len(msList)-1) * 0.99)

	return LatencyStats{
		MinMs: msList[0],
		MaxMs: msList[len(msList)-1],
		AvgMs: sum / float64(len(msList)),
		P50Ms: msList[p50Idx],
		P95Ms: msList[p95Idx],
		P99Ms: msList[p99Idx],
	}
}

type ReliabilityReport struct {
	Release           string                  `json:"release"`
	PID               int                     `json:"pid"`
	CyclesCompleted   int                     `json:"cycles_completed"`
	InitialMemoryMB   float64                 `json:"initial_memory_mb"`
	PeakMemoryMB      float64                 `json:"peak_memory_mb"`
	FinalMemoryMB     float64                 `json:"final_memory_mb"`
	MemoryGrowthMB    float64                 `json:"memory_growth_mb"`
	InitialHandles    uint32                  `json:"initial_handles"`
	PeakHandles       uint32                  `json:"peak_handles"`
	FinalHandles      uint32                  `json:"final_handles"`
	PingStats         LatencyStats            `json:"ping_stats"`
	CycleDurationStats LatencyStats           `json:"cycle_duration_stats"`
	CrashRecoveryMs   float64                 `json:"crash_recovery_ms"`
	Status            string                  `json:"status"`
}

func TestRealNXWarmWorkerReliabilityAndPerformance(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping real NX reliability test on non-Windows host")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("skipping real NX test; set NXGO_RUN_REAL_NX=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	pid := worker.Manifest.PID
	t.Logf("warm worker started with PID %d", pid)

	initMetrics, err := queryProcessMetrics(pid)
	if err != nil {
		t.Logf("warning: queryProcessMetrics: %v", err)
	}
	t.Logf("initial worker metrics: working_set=%.2f MB, handles=%d",
		float64(initMetrics.WorkingSetBytes)/(1024*1024), initMetrics.HandleCount)

	// Baseline session info
	baseInfo, err := session.Info(ctx)
	if err != nil {
		t.Fatalf("session.Info baseline failed: %v", err)
	}
	t.Logf("baseline session info: release=%s thread=%d work_part=%s syslog=%s",
		baseInfo.Release, baseInfo.ThreadID, baseInfo.WorkPart, baseInfo.SyslogPath)

	tempDir := t.TempDir()
	const cycleCount = 3
	var cycleDurations []time.Duration
	var peakMem uint64 = initMetrics.WorkingSetBytes
	var peakHandles uint32 = initMetrics.HandleCount

	for i := 1; i <= cycleCount; i++ {
		start := time.Now()
		partPath := filepath.Join(tempDir, fmt.Sprintf("soak_part_%d.prt", i))
		savedPath := filepath.Join(tempDir, fmt.Sprintf("soak_part_%d_saved.prt", i))
		pdfPath := filepath.Join(tempDir, fmt.Sprintf("soak_sheet_%d.pdf", i))

		part, err := session.NewPart(ctx, partPath, "mm")
		if err != nil {
			t.Fatalf("cycle %d NewPart failed: %v", i, err)
		}

		// Modeling: block + cylinder
		_, err = part.CreateBlock(ctx, nxgo.BlockParams{
			Origin: nxgo.Point3D{0, 0, 0},
			Length: 100,
			Width:  50,
			Height: 25,
		})
		if err != nil {
			t.Fatalf("cycle %d CreateBlock failed: %v", i, err)
		}

		_, err = part.CreateCylinder(ctx, nxgo.CylinderParams{
			Origin:    nxgo.Point3D{50, 25, 25},
			Direction: nxgo.Vector3D{0, 0, 1},
			Diameter:  20,
			Height:    30,
		})
		if err != nil {
			t.Fatalf("cycle %d CreateCylinder failed: %v", i, err)
		}

		massProps, err := part.MassProperties(ctx)
		if err != nil {
			t.Fatalf("cycle %d MassProperties failed: %v", i, err)
		}
		if massProps.Volume <= 0 {
			t.Fatalf("cycle %d expected positive volume, got %.2f", i, massProps.Volume)
		}

		// Drafting: sheet + PDF export
		_, err = part.CreateDrawingSheet(ctx, nxgo.CreateSheetParams{
			SheetName:        fmt.Sprintf("SHEET_%d", i),
			Units:            "mm",
			Height:           297.0,
			Length:           420.0,
			ScaleNumerator:   1.0,
			ScaleDenominator: 1.0,
		})
		if err != nil {
			t.Fatalf("cycle %d CreateDrawingSheet failed: %v", i, err)
		}

		sheets, err := part.DrawingSheets(ctx)
		if err != nil || len(sheets) != 1 {
			t.Fatalf("cycle %d DrawingSheets query failed: err=%v len=%d", i, err, len(sheets))
		}

		pdfRes, err := part.ExportPDF(ctx, nxgo.ExportPDFParams{
			OutputPDFPath: pdfPath,
			SheetNames:    []string{sheets[0].Name},
		})
		if err != nil {
			t.Fatalf("cycle %d ExportPDF failed: %v", i, err)
		}
		if _, err := os.Stat(pdfRes.ExportedPath); err != nil {
			t.Fatalf("cycle %d PDF file missing: %v", i, err)
		}
		if pdfRes.FileSizeBytes <= 0 {
			t.Fatalf("cycle %d expected positive PDF size, got %d", i, pdfRes.FileSizeBytes)
		}

		// SaveAs
		if _, err := part.SaveAs(ctx, savedPath); err != nil {
			t.Fatalf("cycle %d SaveAs failed: %v", i, err)
		}

		// Close
		if err := part.ForceCloseDiscard(ctx); err != nil {
			t.Fatalf("cycle %d ForceCloseDiscard failed: %v", i, err)
		}

		dur := time.Since(start)
		cycleDurations = append(cycleDurations, dur)

		// Sample metrics
		m, _ := queryProcessMetrics(pid)
		if m.WorkingSetBytes > peakMem {
			peakMem = m.WorkingSetBytes
		}
		if m.HandleCount > peakHandles {
			peakHandles = m.HandleCount
		}
		t.Logf("cycle %d completed in %v (current mem: %.2f MB, handles: %d)",
			i, dur.Round(time.Millisecond), float64(m.WorkingSetBytes)/(1024*1024), m.HandleCount)
	}

	// Verify session health after cycles
	endInfo, err := session.Info(ctx)
	if err != nil {
		t.Fatalf("session.Info end query failed: %v", err)
	}
	if endInfo.ThreadID != baseInfo.ThreadID {
		t.Fatalf("worker thread changed: baseline=%d, end=%d", baseInfo.ThreadID, endInfo.ThreadID)
	}

	// Ping RPC latency benchmark
	t.Log("running 25-iteration ping latency probe...")
	var pingDurations []time.Duration
	for i := 0; i < 25; i++ {
		pStart := time.Now()
		if err := session.Ping(ctx); err != nil {
			t.Fatalf("ping %d failed: %v", i, err)
		}
		pingDurations = append(pingDurations, time.Since(pStart))
	}

	finalMetrics, _ := queryProcessMetrics(pid)
	pingStats := computeStats(pingDurations)
	cycleStats := computeStats(cycleDurations)

	// Crash-recovery test
	t.Log("initiating crash-recovery campaign: terminating worker process forcefully...")
	crashStart := time.Now()
	_ = worker.Kill()

	// Verify subsequent RPC on terminated session fails gracefully
	err = session.Ping(ctx)
	if err == nil {
		t.Fatal("expected error calling ping on terminated worker, but call succeeded")
	}
	t.Logf("terminated worker failed gracefully as expected: %v", err)

	// Start fresh worker to verify clean recovery and reuse of pipe resources
	recoveredWorker, recoveredSession := startTestWorker(t, ctx)
	defer recoveredWorker.Kill()

	if err := recoveredSession.Ping(ctx); err != nil {
		t.Fatalf("recovered worker ping failed: %v", err)
	}
	crashRecoveryDur := time.Since(crashStart)
	t.Logf("crash-recovery cycle succeeded in %v (recovered PID %d)", crashRecoveryDur, recoveredWorker.Manifest.PID)

	report := ReliabilityReport{
		Release:            worker.Manifest.NXRelease,
		PID:                pid,
		CyclesCompleted:    cycleCount,
		InitialMemoryMB:    float64(initMetrics.WorkingSetBytes) / (1024 * 1024),
		PeakMemoryMB:       float64(peakMem) / (1024 * 1024),
		FinalMemoryMB:      float64(finalMetrics.WorkingSetBytes) / (1024 * 1024),
		MemoryGrowthMB:     float64(finalMetrics.WorkingSetBytes-initMetrics.WorkingSetBytes) / (1024 * 1024),
		InitialHandles:     initMetrics.HandleCount,
		PeakHandles:        peakHandles,
		FinalHandles:       finalMetrics.HandleCount,
		PingStats:          pingStats,
		CycleDurationStats: cycleStats,
		CrashRecoveryMs:    float64(crashRecoveryDur.Microseconds()) / 1000.0,
		Status:             "PASSED",
	}

	artifactDir := filepath.Join(repoRoot(t), "artifacts", "nx-smoke", t.Name())
	_ = os.MkdirAll(artifactDir, 0755)
	reportBytes, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(filepath.Join(artifactDir, "reliability-report.json"), reportBytes, 0644)

	t.Logf("reliability report generated: cycles=%d mem_delta=%.2f MB ping_p50=%.2f ms ping_p95=%.2f ms recovery=%.1f ms",
		report.CyclesCompleted, report.MemoryGrowthMB, pingStats.P50Ms, pingStats.P95Ms, report.CrashRecoveryMs)
}

func TestRealNXReliabilityAssemblyAndForcedTermination(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping real NX test on non-Windows host")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("skipping real NX test; set NXGO_RUN_REAL_NX=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()

	// 1. Repeated assembly component creation and tree enumeration
	t.Log("running assembly component creation and tree enumeration...")
	pinPath := filepath.Join(tempDir, "rel_pin.prt")
	pinPart, err := session.NewPart(ctx, pinPath, "mm")
	if err != nil {
		t.Fatalf("NewPart pin failed: %v", err)
	}
	_, err = pinPart.CreateCylinder(ctx, nxgo.CylinderParams{
		Origin:    nxgo.Point3D{0, 0, 0},
		Direction: nxgo.Vector3D{0, 0, 1},
		Diameter:  10,
		Height:    30,
	})
	if err != nil {
		t.Fatalf("CreateCylinder failed: %v", err)
	}
	if _, err := pinPart.Save(ctx); err != nil {
		t.Fatalf("Save pin failed: %v", err)
	}
	_ = pinPart.ForceCloseDiscard(ctx)

	// Create root assembly
	assyPath := filepath.Join(tempDir, "rel_assembly.prt")
	assy, err := session.NewPart(ctx, assyPath, "mm")
	if err != nil {
		t.Fatalf("NewPart assy failed: %v", err)
	}

	// Repeatedly add components and verify tree enumeration
	for compIdx := 1; compIdx <= 3; compIdx++ {
		compName := fmt.Sprintf("PIN_COMP_%d", compIdx)
		comp, err := assy.AddComponent(ctx, nxgo.AddComponentParams{
			PartPath:      pinPath,
			ComponentName: compName,
			Origin:        nxgo.Point3D{float64(compIdx * 20), 0, 0},
		})
		if err != nil {
			t.Fatalf("AddComponent %s failed: %v", compName, err)
		}
		if comp.Ref.ObjectID == "" {
			t.Fatalf("expected valid component ref for %s", compName)
		}
		t.Logf("component %s added: handle=%s name=%s", compName, comp.Ref.ObjectID, comp.Name)

		tree, err := assy.ComponentTree(ctx)
		if err != nil {
			t.Fatalf("ComponentTree failed at component %d: %v", compIdx, err)
		}
		if len(tree.Children) != compIdx {
			t.Fatalf("expected %d children in tree, got %d", compIdx, len(tree.Children))
		}
		t.Logf("tree verified at depth 1 with %d children", len(tree.Children))
	}
	_ = assy.ForceCloseDiscard(ctx)

	// 2. Forced NX termination at mutation phase
	t.Log("testing forced NX termination during active mutation phase...")
	mutPartPath := filepath.Join(tempDir, "mutation_kill.prt")
	mutPart, err := session.NewPart(ctx, mutPartPath, "mm")
	if err != nil {
		t.Fatalf("NewPart for mutation kill failed: %v", err)
	}

	// Trigger async kill shortly after launching a mutation
	killTimer := time.AfterFunc(15*time.Millisecond, func() {
		_ = worker.Kill()
	})
	defer killTimer.Stop()

	// This mutation should be cut off by worker kill and fail fast
	_, mutErr := mutPart.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 500,
		Width:  500,
		Height: 500,
	})
	if mutErr == nil {
		t.Log("mutation finished before kill signal; killing now and verifying next call fails")
		_ = worker.Kill()
		mutErr = session.Ping(ctx)
	}
	if mutErr == nil {
		t.Fatal("expected error after worker termination, but call succeeded")
	}
	t.Logf("forced mutation termination handled cleanly: %v", mutErr)

	// 3. Verify clean recreation of worker after abrupt mutation termination
	t.Log("verifying fresh worker starts cleanly after mutation phase crash...")
	freshWorker, freshSession := startTestWorker(t, ctx)
	defer freshWorker.Kill()

	freshInfo, err := freshSession.Info(ctx)
	if err != nil {
		t.Fatalf("fresh session info failed: %v", err)
	}
	t.Logf("fresh worker operational: release=%s thread=%d", freshInfo.Release, freshInfo.ThreadID)
}

