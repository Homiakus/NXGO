package supervisor

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	reqCorrelationRegex     = regexp.MustCompile(`\[nxgo:req:([A-Za-z0-9_-]+)\]`)
	txCorrelationRegex      = regexp.MustCompile(`\[nxgo:tx:([A-Za-z0-9_-]+)\]`)
	sessionCorrelationRegex = regexp.MustCompile(`\[nxgo:session:([A-Za-z0-9_-]+)\]`)
)

// CorrelatedSyslogEntry represents a parsed NX syslog entry correlated with NXGO execution IDs.
type CorrelatedSyslogEntry struct {
	Timestamp string `json:"timestamp,omitempty"`
	Level     string `json:"level,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	TxID      string `json:"tx_id,omitempty"`
	Message   string `json:"message"`
	RawLine   string `json:"raw_line"`
}

type SyslogCollector struct {
	SyslogPath string
}

func NewSyslogCollector(syslogPath string) *SyslogCollector {
	return &SyslogCollector{SyslogPath: syslogPath}
}

func (c *SyslogCollector) ReadRecentLines(maxBytes int64) ([]byte, error) {
	if c.SyslogPath == "" {
		return nil, fmt.Errorf("syslog path is empty")
	}
	f, err := os.Open(c.SyslogPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := fi.Size()
	offset := int64(0)
	if size > maxBytes {
		offset = size - maxBytes
	}

	buf := make([]byte, size-offset)
	_, err = f.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *SyslogCollector) ExportToArtifactDir(artifactDir string, filename string) (string, error) {
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return "", err
	}
	if filename == "" {
		filename = fmt.Sprintf("nx_syslog_%s.log", time.Now().Format("20060102_150405"))
	}
	destPath := filepath.Join(artifactDir, filename)

	data, err := os.ReadFile(c.SyslogPath)
	if err != nil {
		return "", fmt.Errorf("failed reading source syslog: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed writing merged syslog to artifact dir: %w", err)
	}
	return destPath, nil
}

// ExtractCorrelations parses syslog bytes and extracts structured correlated entries.
func ExtractCorrelations(data []byte) []CorrelatedSyslogEntry {
	var entries []CorrelatedSyslogEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		entry := CorrelatedSyslogEntry{
			RawLine: line,
			Message: trimmed,
		}

		if match := reqCorrelationRegex.FindStringSubmatch(line); len(match) > 1 {
			entry.RequestID = match[1]
		}
		if match := txCorrelationRegex.FindStringSubmatch(line); len(match) > 1 {
			entry.TxID = match[1]
		}
		if match := sessionCorrelationRegex.FindStringSubmatch(line); len(match) > 1 {
			entry.SessionID = match[1]
		}

		// Check for common log severity tags
		switch {
		case strings.Contains(line, ">>> ERROR") || strings.Contains(line, "Fatal"):
			entry.Level = "ERROR"
		case strings.Contains(line, ">>> WARNING") || strings.Contains(line, "Warn"):
			entry.Level = "WARN"
		case strings.Contains(line, ">>> INFO"):
			entry.Level = "INFO"
		default:
			entry.Level = "DEBUG"
		}

		entries = append(entries, entry)
	}

	return entries
}

// FilterByRequestID filters a list of entries matching a specific RequestID.
func FilterByRequestID(entries []CorrelatedSyslogEntry, reqID string) []CorrelatedSyslogEntry {
	var filtered []CorrelatedSyslogEntry
	for _, e := range entries {
		if e.RequestID == reqID {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// FilterByTxID filters a list of entries matching a specific TxID.
func FilterByTxID(entries []CorrelatedSyslogEntry, txID string) []CorrelatedSyslogEntry {
	var filtered []CorrelatedSyslogEntry
	for _, e := range entries {
		if e.TxID == txID {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
