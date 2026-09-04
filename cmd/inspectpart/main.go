package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

type PartInspectionReport struct {
	PartPath        string                   `json:"part_path"`
	Name            string                   `json:"name"`
	Units           string                   `json:"units"`
	Summary         *protocol.PartSummaryResponse `json:"summary,omitempty"`
	Attributes      []protocol.PartAttribute `json:"attributes"`
	Expressions     []ExpressionInfo         `json:"expressions"`
	MassProperties  *nxgo.MassProperties     `json:"mass_properties,omitempty"`
	BoundingBox     *nxgo.BoundingBox        `json:"bounding_box,omitempty"`
}

type ExpressionInfo struct {
	Name        string  `json:"name"`
	Formula     string  `json:"formula"`
	Value       float64 `json:"value"`
	StringValue string  `json:"string_value,omitempty"`
	Type        string  `json:"type"`
	Units       string  `json:"units,omitempty"`
}

func main() {
	defaultPart := `C:\Users\KDFX Modes\Documents\vault_v2.0\20-Projects\201-Engineering-Projects\PKL.7.14\01-CAD\NX\PKL-ASM-000-V1.2.prt`
	partPathFlag := flag.String("part", defaultPart, "Path to .prt file to inspect")
	flag.Parse()

	partPath := *partPathFlag
	if _, err := os.Stat(partPath); err != nil {
		fmt.Fprintf(os.Stderr, "Part file not found: %s (%v)\n", partPath, err)
		os.Exit(1)
	}

	fmt.Printf("Starting real NX worker to inspect: %s\n", partPath)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	repoRoot, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve root: %v\n", err)
		os.Exit(1)
	}

	agentBin := filepath.Join(repoRoot, "agent", "bin")

	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		AgentMode:      supervisor.AgentModeCanonical,
		AgentBin:       agentBin,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start NX worker: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("Stopping NX worker...")
		_ = worker.Kill()
	}()

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)

	if err := session.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Session ping failed: %v\n", err)
		os.Exit(1)
	}

	info, err := session.Info(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Session info failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Connected to NX %s (Session: %s, Thread: %d)\n", info.Release, info.SessionID, info.ThreadID)

	fmt.Printf("Opening part: %s ...\n", partPath)
	part, err := session.OpenPart(ctx, partPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenPart failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Part opened successfully: %s (Units: %s)\n", part.Name, part.Units)

	report := PartInspectionReport{
		PartPath: partPath,
		Name:     part.Name,
		Units:    part.Units,
	}

	// 1. Summary
	summary, err := part.Summary(ctx)
	if err == nil {
		report.Summary = summary
		fmt.Printf("Summary: Bodies=%d, Features=%d, Components=%d\n",
			summary.BodyCount, summary.FeatureCount, summary.ComponentCount)
	} else {
		fmt.Printf("Summary query warning: %v\n", err)
	}

	// 2. Attributes
	attrs, err := part.GetAttributes(ctx)
	if err == nil {
		report.Attributes = attrs
		fmt.Printf("Attributes found: %d\n", len(attrs))
		for _, a := range attrs {
			fmt.Printf("  - [%s] %s = %v\n", a.Type, a.Title, a.Value)
		}
	} else {
		fmt.Printf("Attributes query warning: %v\n", err)
	}

	// 3. Expressions
	exprs, err := part.ListExpressions(ctx)
	if err == nil {
		fmt.Printf("Expressions / Parameters found: %d\n", len(exprs))
		for _, e := range exprs {
			report.Expressions = append(report.Expressions, ExpressionInfo{
				Name:        e.Name,
				Formula:     e.Formula,
				Value:       e.Value,
				StringValue: e.StringValue,
				Type:        e.Type,
				Units:       e.Units,
			})
			fmt.Printf("  - %s = %s (value: %v, units: %s)\n", e.Name, e.Formula, e.Value, e.Units)
		}
	} else {
		fmt.Printf("Expressions query warning: %v\n", err)
	}

	// 4. Mass properties & Bounding box (if bodies exist)
	if summary != nil && summary.BodyCount > 0 {
		mp, err := part.MassProperties(ctx)
		if err == nil {
			report.MassProperties = mp
			fmt.Printf("Mass Properties: Volume=%.2f mm3, Area=%.2f mm2, Mass=%.4f kg\n", mp.Volume, mp.Area, mp.Mass)
		}

		bbox, err := part.BoundingBox(ctx)
		if err == nil {
			report.BoundingBox = bbox
			fmt.Printf("Bounding Box: Dimensions=[%.2f, %.2f, %.2f] mm\n",
				bbox.Dimensions[0], bbox.Dimensions[1], bbox.Dimensions[2])
		}
	}

	// 5. Existing Drawing Sheets
	sheets, err := part.DrawingSheets(ctx)
	if err == nil {
		fmt.Printf("Drawing sheets found: %d\n", len(sheets))
		for _, s := range sheets {
			fmt.Printf("  - Sheet %q (Size: %.1f x %.1f mm)\n", s.Name, s.Length, s.Height)
		}
	} else {
		fmt.Printf("Drawing sheets query warning: %v\n", err)
	}

	// Output complete JSON report
	outFile := filepath.Join(repoRoot, "artifacts", "inspected_part.json")
	_ = os.MkdirAll(filepath.Dir(outFile), 0755)
	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(outFile, data, 0644)
	fmt.Printf("\nFull inspection saved to: %s\n", outFile)
}
