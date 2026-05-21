package data

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
)

type memorySessionChangeStore struct {
	mu      sync.RWMutex
	byTask  map[string]map[string]*sessionFileState
	baseDir string
}

type sessionFileState struct {
	Path       string
	Baseline   string
	Current    string
	Status     string
	Operations []biz.SessionFileOp
	UpdatedAt  time.Time
}

func NewSessionChangeStore(dataConf *conf.Data) biz.SessionChangeStore {
	dir := filepath.Join(defaultMemoryDir, "changes")
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
	return &memorySessionChangeStore{
		byTask:  make(map[string]map[string]*sessionFileState),
		baseDir: dir,
	}
}

func (s *memorySessionChangeStore) RecordChange(taskID, path, toolName, operation, summary, before, after string) {
	taskID = strings.TrimSpace(taskID)
	path = strings.TrimSpace(path)
	if taskID == "" || path == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	taskChanges := s.byTask[taskID]
	if taskChanges == nil {
		taskChanges = make(map[string]*sessionFileState)
		s.byTask[taskID] = taskChanges
	}
	state, ok := taskChanges[path]
	if !ok {
		state = &sessionFileState{
			Path:      path,
			Baseline:  before,
			Current:   after,
			Status:    classifyFileChangeStatus(before, after),
			UpdatedAt: time.Now(),
		}
		taskChanges[path] = state
	} else {
		state.Current = after
		state.Status = classifyFileChangeStatus(state.Baseline, after)
		state.UpdatedAt = time.Now()
	}
	state.Operations = append(state.Operations, biz.SessionFileOp{
		Time:      time.Now(),
		Tool:      toolName,
		Operation: operation,
		Summary:   summary,
	})
	s.persistLocked(taskID)
}

func (s *memorySessionChangeStore) Snapshot(taskID string) []biz.SessionFileChange {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	taskChanges := s.byTask[taskID]
	if len(taskChanges) == 0 {
		return s.loadPersistedLocked(taskID)
	}

	paths := make([]string, 0, len(taskChanges))
	for path := range taskChanges {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := make([]biz.SessionFileChange, 0, len(paths))
	for _, path := range paths {
		state := taskChanges[path]
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

func cloneSessionFileChange(state *sessionFileState) biz.SessionFileChange {
	if state == nil {
		return biz.SessionFileChange{}
	}
	return biz.SessionFileChange{
		Path:        state.Path,
		Status:      state.Status,
		Baseline:    state.Baseline,
		Current:     state.Current,
		UnifiedDiff: formatUnifiedDiff(state.Path, state.Baseline, state.Current),
		Operations:  append([]biz.SessionFileOp(nil), state.Operations...),
		UpdatedAt:   state.UpdatedAt,
	}
}

func (s *memorySessionChangeStore) persistLocked(taskID string) {
	if s.baseDir == "" {
		return
	}
	taskChanges := s.byTask[taskID]
	if len(taskChanges) == 0 {
		return
	}
	paths := make([]string, 0, len(taskChanges))
	for path := range taskChanges {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	snapshot := make([]biz.SessionFileChange, 0, len(paths))
	for _, path := range paths {
		snapshot = append(snapshot, cloneSessionFileChange(taskChanges[path]))
	}
	raw, err := json.MarshalIndent(map[string]any{
		"task_id": taskID,
		"files":   snapshot,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.baseDir, safeFileName(taskID)+".json"), append(raw, '\n'), 0o644)
}

func (s *memorySessionChangeStore) loadPersistedLocked(taskID string) []biz.SessionFileChange {
	if s.baseDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(s.baseDir, safeFileName(taskID)+".json"))
	if err != nil {
		return nil
	}
	var payload struct {
		Files []biz.SessionFileChange `json:"files"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload.Files
}
