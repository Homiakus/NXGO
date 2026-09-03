package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrNoInstallations = errors.New("no valid Siemens NX installations found")
	ErrVersionNotFound = errors.New("requested NX version not found")
)

var versionRE = regexp.MustCompile(`NX\s*(\d{4})|(\d{4})`)

type Installation struct {
	Release             string `json:"release"`
	Home                string `json:"home"`
	UGII                string `json:"ugii"`
	RunJournal          string `json:"run_journal"`
	RunDotnetCoreNXOpen string `json:"run_dotnet_core_nxopen"`
	ManagedDir          string `json:"managed_dir"`
	NXOpenDLL           string `json:"nxopen_dll"`
	HasNXOpenUF         bool   `json:"has_nxopen_uf"`
	Source              string `json:"source"`
}

func InspectInstallation(path string, source string) (*Installation, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("directory %q does not exist or is not a folder", clean)
	}

	// Locate run_journal.exe in UGII or NXBIN
	runJournal := filepath.Join(clean, "UGII", "run_journal.exe")
	ugii := filepath.Join(clean, "UGII")
	if _, err := os.Stat(runJournal); err != nil {
		altJournal := filepath.Join(clean, "NXBIN", "run_journal.exe")
		if _, err := os.Stat(altJournal); err == nil {
			runJournal = altJournal
			ugii = filepath.Join(clean, "NXBIN")
		} else {
			return nil, fmt.Errorf("run_journal.exe not found in %s/UGII or NXBIN", clean)
		}
	}

	// NX2512's supported .NET 8 path is managed_core. Keep the legacy managed
	// fallback for discovery and the Python smoke, but never select it for the
	// canonical Agent launch below.
	managedDir := ""
	for _, candidate := range []string{
		filepath.Join(clean, "NXBIN", "managed_core"),
		filepath.Join(clean, "UGII", "managed_core"),
		filepath.Join(clean, "NXBIN", "managed"),
		filepath.Join(clean, "UGII", "managed"),
	} {
		if _, err := os.Stat(filepath.Join(candidate, "NXOpen.dll")); err == nil {
			managedDir = candidate
			break
		}
	}
	if managedDir == "" {
		// Also check direct UGII/NXBIN for older installations.
		for _, candidate := range []string{ugii, filepath.Join(clean, "NXBIN")} {
			if _, err := os.Stat(filepath.Join(candidate, "NXOpen.dll")); err == nil {
				managedDir = candidate
				break
			}
		}
	}
	if managedDir == "" {
		return nil, fmt.Errorf("NXOpen.dll not found in %s/managed_core or %s/managed", clean, ugii)
	}
	nxopenDLL := filepath.Join(managedDir, "NXOpen.dll")

	runDotnetCore := ""
	for _, candidate := range []string{
		filepath.Join(clean, "NXBIN", "managed_core", "run_dotnet_core_nxopen.exe"),
		filepath.Join(clean, "UGII", "managed_core", "run_dotnet_core_nxopen.exe"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			runDotnetCore = candidate
			break
		}
	}

	hasUF := false
	ufDLL := filepath.Join(filepath.Dir(nxopenDLL), "NXOpen.UF.dll")
	if _, err := os.Stat(ufDLL); err == nil {
		hasUF = true
	}

	release := parseRelease(clean)

	return &Installation{
		Release:             release,
		Home:                clean,
		UGII:                ugii,
		RunJournal:          runJournal,
		RunDotnetCoreNXOpen: runDotnetCore,
		ManagedDir:          managedDir,
		NXOpenDLL:           nxopenDLL,
		HasNXOpenUF:         hasUF,
		Source:              source,
	}, nil
}

func parseRelease(path string) string {
	matches := versionRE.FindStringSubmatch(filepath.Base(path))
	if len(matches) > 1 && matches[1] != "" {
		return matches[1]
	}
	if len(matches) > 2 && matches[2] != "" {
		return matches[2]
	}
	// Try full path
	all := versionRE.FindAllStringSubmatch(path, -1)
	if len(all) > 0 {
		last := all[len(all)-1]
		if len(last) > 1 && last[1] != "" {
			return last[1]
		}
		if len(last) > 2 && last[2] != "" {
			return last[2]
		}
	}
	return "unknown"
}

func Discover(customRoots ...string) ([]Installation, error) {
	if len(customRoots) > 0 {
		return DiscoverRoots(false, customRoots...)
	}
	return DiscoverRoots(true)
}

func DiscoverRoots(includeSystem bool, customRoots ...string) ([]Installation, error) {
	var results []Installation
	seen := map[string]bool{}

	checkAdd := func(path string, source string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		if inst, err := InspectInstallation(clean, source); err == nil {
			seen[clean] = true
			results = append(results, *inst)
		}
	}

	for _, root := range customRoots {
		checkAdd(root, "argument")
	}

	if includeSystem {
		if envHome := strings.TrimSpace(os.Getenv("NXGO_NX_HOME")); envHome != "" {
			checkAdd(envHome, "env:NXGO_NX_HOME")
		}
		if envUGII := strings.TrimSpace(os.Getenv("UGII_BASE_DIR")); envUGII != "" {
			checkAdd(envUGII, "env:UGII_BASE_DIR")
		}
		if matrix := strings.TrimSpace(os.Getenv("NXGO_NX_MATRIX")); matrix != "" {
			for _, part := range strings.Split(matrix, ";") {
				checkAdd(strings.TrimSpace(part), "env:NXGO_NX_MATRIX")
			}
		}

		// Standard Siemens install directories on Windows
		programFiles := os.Getenv("ProgramFiles")
		if programFiles != "" {
			siemensDir := filepath.Join(programFiles, "Siemens")
			if entries, err := os.ReadDir(siemensDir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() && strings.HasPrefix(entry.Name(), "NX") {
						checkAdd(filepath.Join(siemensDir, entry.Name()), "standard:ProgramFiles")
					}
				}
			}
		}
	}

	if len(results) == 0 {
		return nil, ErrNoInstallations
	}
	return results, nil
}

func SelectVersion(requested string, customRoots ...string) (*Installation, error) {
	list, err := Discover(customRoots...)
	if err != nil {
		return nil, err
	}

	if requested == "" {
		return &list[0], nil
	}

	for _, inst := range list {
		if inst.Release == requested || strings.EqualFold(inst.Release, requested) {
			return &inst, nil
		}
	}
	return nil, fmt.Errorf("%w: %q (available: %s)", ErrVersionNotFound, requested, availableVersions(list))
}

func availableVersions(list []Installation) string {
	var names []string
	for _, inst := range list {
		names = append(names, fmt.Sprintf("%s (%s)", inst.Release, inst.Home))
	}
	return strings.Join(names, ", ")
}
