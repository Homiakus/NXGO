package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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
