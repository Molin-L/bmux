package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Molin-L/bmux/internal/model"
)

const (
	activeStatus = "active"
)

type fileState struct {
	Tasks map[string]model.TaskBranchMeta `json:"tasks"`
}

type Store struct {
	mu   sync.Mutex
	path string
}

func New(projectRoot string) *Store {
	return &Store{path: filepath.Join(projectRoot, ".bmux", "state.json")}
}

func (s *Store) Upsert(meta model.TaskBranchMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if meta.IssueID == "" {
		return errors.New("issue id is required")
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now().UTC()
	}
	if meta.Status == "" {
		meta.Status = activeStatus
	}

	st, err := s.readLocked()
	if err != nil {
		return err
	}
	st.Tasks[meta.IssueID] = meta
	return s.writeLocked(st)
}

func (s *Store) ByIssueID(issueID string) (model.TaskBranchMeta, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.readLocked()
	if err != nil {
		return model.TaskBranchMeta{}, false, err
	}
	meta, ok := st.Tasks[issueID]
	return meta, ok, nil
}

func (s *Store) All() ([]model.TaskBranchMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	out := make([]model.TaskBranchMeta, 0, len(st.Tasks))
	for _, meta := range st.Tasks {
		out = append(out, meta)
	}
	return out, nil
}

func (s *Store) MarkStatus(issueID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.readLocked()
	if err != nil {
		return err
	}
	meta, ok := st.Tasks[issueID]
	if !ok {
		return fmt.Errorf("issue %s not found in state", issueID)
	}
	meta.Status = status
	st.Tasks[issueID] = meta
	return s.writeLocked(st)
}

func (s *Store) readLocked() (fileState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileState{Tasks: map[string]model.TaskBranchMeta{}}, nil
		}
		return fileState{}, fmt.Errorf("read state: %w", err)
	}

	var st fileState
	if err := json.Unmarshal(data, &st); err != nil {
		return fileState{}, fmt.Errorf("parse state: %w", err)
	}
	if st.Tasks == nil {
		st.Tasks = map[string]model.TaskBranchMeta{}
	}
	return st, nil
}

func (s *Store) writeLocked(st fileState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
