package supervisor

import (
	"regexp"
	"strings"
)

// CrashKind categorizes known fatal process failure signatures.
type CrashKind string

const (
	CrashKindAccessViolation          CrashKind = "ACCESS_VIOLATION"
	CrashKindStackOverflow            CrashKind = "STACK_OVERFLOW"
	CrashKindLicenseExpired           CrashKind = "LICENSE_EXPIRED_OR_UNAVAILABLE"
	CrashKindNXOpenUnhandledException CrashKind = "NXOPEN_UNHANDLED_EXCEPTION"
	CrashKindMissingDLL               CrashKind = "MISSING_DLL_DEPENDENCY"
	CrashKindDotnetPanic              CrashKind = "DOTNET_RUNTIME_PANIC"
	CrashKindPipeBroken               CrashKind = "PIPE_BROKEN"
	CrashKindTimeout                  CrashKind = "PROCESS_TIMEOUT"
	CrashKindUnknown                  CrashKind = "UNKNOWN_FATAL_CRASH"
)

// CrashReport summarizes automated classification of a failed or aborted worker process.
type CrashReport struct {
	Kind          CrashKind `json:"kind"`
	ExitCode      int       `json:"exit_code"`
	FatalMessage  string    `json:"fatal_message"`
	CulpritSymbol string    `json:"culprit_symbol,omitempty"`
	StackSnippet  string    `json:"stack_snippet,omitempty"`
	SyslogErrors  []string  `json:"syslog_errors,omitempty"`
}

var (
	accessViolationRegex = regexp.MustCompile(`(?i)(Access Violation|0xC0000005|c0000005|EXCEPTION_ACCESS_VIOLATION)`)
	stackOverflowRegex   = regexp.MustCompile(`(?i)(Stack overflow|0xC00000FD|c00000fd|EXCEPTION_STACK_OVERFLOW)`)
	licenseRegex         = regexp.MustCompile(`(?i)(License not available|SPLM_LICENSE_SERVER|license error|Cannot connect to license server|-15,-103)`)
	missingDLLRegex      = regexp.MustCompile(`(?i)(DllNotFoundException|Unable to load DLL|The specified module could not be found|0xC0000135)`)
	dotnetPanicRegex     = regexp.MustCompile(`(?i)(Unhandled exception|System\..*Exception|Fatal error in .NET Runtime)`)
	symbolRegex          = regexp.MustCompile(`(?i)at (NXOpen\.[A-Za-z0-9_.]+|NXGO\.Agent\.[A-Za-z0-9_.]+|[A-Za-z0-9_]+\.dll)`)
)

// ClassifyCrash analyzes runner process output, syslog content, and process exit code to identify root cause.
func ClassifyCrash(runnerOutput string, syslogOutput string, exitCode int) *CrashReport {
	combined := runnerOutput + "\n" + syslogOutput

	report := &CrashReport{
		Kind:     CrashKindUnknown,
		ExitCode: exitCode,
	}

	// Extract syslog error lines
	var syslogErrors []string
	for _, line := range strings.Split(syslogOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, ">>> ERROR") || strings.Contains(trimmed, "Fatal error") || strings.Contains(trimmed, "Exception:") {
			syslogErrors = append(syslogErrors, trimmed)
		}
	}
	report.SyslogErrors = syslogErrors

	// Identify culprit symbol/stack if present
	if match := symbolRegex.FindStringSubmatch(runnerOutput); len(match) > 1 {
		report.CulpritSymbol = match[1]
	}

	// Pattern match signatures in order of specificity
	switch {
	case accessViolationRegex.MatchString(combined) || exitCode == -1073741819 || exitCode == 3221225477:
		report.Kind = CrashKindAccessViolation
		report.FatalMessage = "Memory access violation encountered in native NX or C# runtime"
	case stackOverflowRegex.MatchString(combined) || exitCode == -1073741571 || exitCode == 3221225725:
		report.Kind = CrashKindStackOverflow
		report.FatalMessage = "Stack overflow encountered during recursive NXOpen operation"
	case licenseRegex.MatchString(combined):
		report.Kind = CrashKindLicenseExpired
		report.FatalMessage = "Siemens PLM license server unavailable or feature license checkout failed"
	case missingDLLRegex.MatchString(combined):
		report.Kind = CrashKindMissingDLL
		report.FatalMessage = "Required native NX or managed DLL was not found in UGII_NXBIN or assembly path"
	case strings.Contains(combined, "NXOpen.NXException"):
		report.Kind = CrashKindNXOpenUnhandledException
		report.FatalMessage = "Unhandled NXException thrown by Siemens NXOpen builder"
	case dotnetPanicRegex.MatchString(combined):
		report.Kind = CrashKindDotnetPanic
		report.FatalMessage = "Unhandled .NET exception or CLR runtime crash"
	default:
		if exitCode != 0 {
			report.FatalMessage = "Process exited with non-zero exit status without recognizable signature"
		}
	}

	// Capture brief stack snippet
	lines := strings.Split(runnerOutput, "\n")
	var stackLines []string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") || strings.Contains(line, "Exception:") {
			stackLines = append(stackLines, strings.TrimSpace(line))
			if len(stackLines) >= 5 {
				break
			}
		}
	}
	if len(stackLines) > 0 {
		report.StackSnippet = strings.Join(stackLines, "\n")
	}

	return report
}
