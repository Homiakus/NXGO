package supervisor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrInvalidManifest = errors.New("invalid worker manifest")
)

type WorkerManifest struct {
	ID          string    `json:"id"`
	PID         int       `json:"pid"`
	NXHome      string    `json:"nx_home"`
	NXRelease   string    `json:"nx_release"`
	Endpoint    string    `json:"endpoint"`
	StartedAt   time.Time `json:"started_at"`
	Owner       string    `json:"owner"`
	Mode        string    `json:"mode"`
	ArtifactDir string    `json:"artifact_dir"`
}

func (m *WorkerManifest) Validate() error {
	if m.ID == "" || m.PID <= 0 || m.NXHome == "" || m.Endpoint == "" {
		return fmt.Errorf("%w: missing required fields (id, pid, nx_home, endpoint)", ErrInvalidManifest)
	}
	return nil
}

func SaveManifest(path string, m *WorkerManifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write tmp manifest: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("commit manifest: %w", err)
	}
	return nil
}

func LoadManifest(path string) (*WorkerManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m WorkerManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
