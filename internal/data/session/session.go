package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	"kratos-demo/internal/data/common"
)

type Store struct {
	mu      sync.RWMutex
	byID    map[string]*sessionImpl
	baseDir string
}

type sessionImpl struct {
	id    string
	store *Store
	mu    sync.Mutex
	files map[string]*sessionFileState
}

type sessionFileState struct {
	Path       string
	Baseline   string
	Current    string
	Status     string
	Operations []SessionFileOp
	UpdatedAt  time.Time
}

func NewSessionStore(dataConf *conf.Data) SessionStore {
	dir := filepath.Join(public.DefaultMemoryDir, "changes")
	if dataConf != nil && dataConf.GetAgentMemory() != nil {
		if configured := strings.TrimSpace(dataConf.GetAgentMemory().GetDir()); configured != "" {
			dir = filepath.Join(configured, "changes")
		}
	}
	workspace, err := os.Getwd()
	if err == nil && !filepath.IsAbs(dir) {
		dir = filepath.Join(workspace, dir)
	}
	_ = os.MkdirAll(dir, 0o755)
	return &Store{
		byID:    make(map[string]*sessionImpl),
		baseDir: dir,
	}
}

func (s *Store) Open(sessionID string) Session {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byID[sessionID]; ok {
		return sess
	}
	sess := &sessionImpl{
		id:    sessionID,
		store: s,
		files: make(map[string]*sessionFileState),
	}
	s.byID[sessionID] = sess
	return sess
}

func (s *sessionImpl) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *sessionImpl) RecordFileChange(path, toolName, operation, summary, before, after string) {
	if s == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.files[path]
	if !ok {
		state = &sessionFileState{
			Path:      path,
			Baseline:  before,
			Current:   after,
			Status:    classifyFileChangeStatus(before, after),
			UpdatedAt: time.Now(),
		}
		s.files[path] = state
	} else {
		state.Current = after
		state.Status = classifyFileChangeStatus(state.Baseline, after)
		state.UpdatedAt = time.Now()
	}
	state.Operations = append(state.Operations, SessionFileOp{
		Time:      time.Now(),
		Tool:      toolName,
		Operation: operation,
		Summary:   summary,
	})
	s.persistLocked()
}

func (s *sessionImpl) FileChanges() []SessionFileChange {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.files) == 0 {
		return s.loadPersistedLocked()
	}

	paths := make([]string, 0, len(s.files))
	for path := range s.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := make([]SessionFileChange, 0, len(paths))
	for _, path := range paths {
		state := s.files[path]
		if state == nil || state.Status == "unchanged" {
			continue
		}
		result = append(result, cloneSessionFileChange(state))
	}
	return result
}

func classifyFileChangeStatus(before, after string) string {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	switch {
	case before == "" && after != "":
		return "created"
	case before != "" && after == "":
		return "deleted"
	case before != after:
		return "modified"
	default:
		return "unchanged"
	}
}

func cloneSessionFileChange(state *sessionFileState) SessionFileChange {
	if state == nil {
		return SessionFileChange{}
	}
	return SessionFileChange{
		Path:        state.Path,
		Status:      state.Status,
		Baseline:    state.Baseline,
		Current:     state.Current,
		UnifiedDiff: common.FormatUnifiedDiff(state.Path, state.Baseline, state.Current),
		Operations:  append([]SessionFileOp(nil), state.Operations...),
		UpdatedAt:   state.UpdatedAt,
	}
}

func (s *sessionImpl) persistLocked() {
	if s == nil || s.store == nil || s.store.baseDir == "" || s.id == "" {
		return
	}
	if len(s.files) == 0 {
		return
	}
	paths := make([]string, 0, len(s.files))
	for path := range s.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	snapshot := make([]SessionFileChange, 0, len(paths))
	for _, path := range paths {
		snapshot = append(snapshot, cloneSessionFileChange(s.files[path]))
	}
	raw, err := json.MarshalIndent(map[string]any{
		"task_id": s.id,
		"files":   snapshot,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.store.baseDir, common.SafeFileName(s.id)+".json"), append(raw, '\n'), 0o644)
}

func (s *sessionImpl) loadPersistedLocked() []SessionFileChange {
	if s == nil || s.store == nil || s.store.baseDir == "" || s.id == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(s.store.baseDir, common.SafeFileName(s.id)+".json"))
	if err != nil {
		return nil
	}
	var payload struct {
		Files []SessionFileChange `json:"files"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload.Files
}
