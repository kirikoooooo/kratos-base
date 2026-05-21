package data

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultMemoryDir    = ".kratos/agent"
	defaultMemoryUserID = "default"
)

type fileAgentMemoryStore struct {
	baseDir string
	userID  string
	mu      sync.Mutex
	log     *log.Helper
}

func NewAgentMemoryStore(dataConf *conf.Data, logger log.Logger) (biz.AgentMemoryStore, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	dir := defaultMemoryDir
	userID := defaultMemoryUserID
	if dataConf != nil && dataConf.GetAgentMemory() != nil {
		if configured := strings.TrimSpace(dataConf.GetAgentMemory().GetDir()); configured != "" {
			dir = configured
		}
		if configuredUser := strings.TrimSpace(dataConf.GetAgentMemory().GetUserId()); configuredUser != "" {
			userID = configuredUser
		}
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(workspace, dir)
	}

	store := &fileAgentMemoryStore{
		baseDir: dir,
		userID:  userID,
		log:     log.NewHelper(logger),
	}
	if err := store.ensureLayout(); err != nil {
		return nil, err
	}
	return store, nil
}

func NewAgentMemoryConfig(dataConf *conf.Data) biz.AgentMemoryConfig {
	dir := defaultMemoryDir
	userID := defaultMemoryUserID
	threshold := 0
	keepRecent := 0
	toolOutputMax := 0
	if dataConf != nil && dataConf.GetAgentMemory() != nil {
		am := dataConf.GetAgentMemory()
		if configured := strings.TrimSpace(am.GetDir()); configured != "" {
			dir = configured
		}
		if configuredUser := strings.TrimSpace(am.GetUserId()); configuredUser != "" {
			userID = configuredUser
		}
		threshold = int(am.GetContextCompressThreshold())
		keepRecent = int(am.GetKeepRecentTurns())
		toolOutputMax = int(am.GetToolOutputMaxChars())
	}
	return biz.AgentMemoryConfig{
		Dir:                      dir,
		UserID:                   userID,
		ContextCompressThreshold: threshold,
		KeepRecentTurns:          keepRecent,
		ToolOutputMaxChars:       toolOutputMax,
	}
}

func (s *fileAgentMemoryStore) ensureLayout() error {
	for _, sub := range []string{"users", "sessions", "conversations", "errors"} {
		if err := os.MkdirAll(filepath.Join(s.baseDir, sub), 0o755); err != nil {
			return fmt.Errorf("create memory dir %s: %w", sub, err)
		}
	}
	return nil
}

func (s *fileAgentMemoryStore) LoadUser(_ context.Context, userID string) (*biz.UserAgentMemory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userID = s.normalizeUserID(userID)
	path := s.userPath(userID)
	memory, err := s.readUserFile(path)
	if err != nil {
		return nil, err
	}
	if memory == nil {
		memory = &biz.UserAgentMemory{UserID: userID}
	}
	memory.UserID = userID
	return memory, nil
}

func (s *fileAgentMemoryStore) SaveUser(_ context.Context, memory *biz.UserAgentMemory) error {
	if memory == nil {
		return errors.New("user memory is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	memory.UserID = s.normalizeUserID(memory.UserID)
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = time.Now()
	}
	return s.writeJSON(s.userPath(memory.UserID), memory)
}

func (s *fileAgentMemoryStore) LoadSession(_ context.Context, sessionID string) (*biz.SessionAgentMemory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	path := s.sessionPath(sessionID)
	memory, err := s.readSessionFile(path)
	if err != nil {
		return nil, err
	}
	if memory == nil {
		memory = &biz.SessionAgentMemory{SessionID: sessionID}
	}
	memory.SessionID = sessionID
	return memory, nil
}

func (s *fileAgentMemoryStore) SaveSession(_ context.Context, memory *biz.SessionAgentMemory) error {
	if memory == nil {
		return errors.New("session memory is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	memory.SessionID = strings.TrimSpace(memory.SessionID)
	if memory.SessionID == "" {
		return errors.New("session id is required")
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = time.Now()
	}
	return s.writeJSON(s.sessionPath(memory.SessionID), memory)
}

func (s *fileAgentMemoryStore) userPath(userID string) string {
	return filepath.Join(s.baseDir, "users", safeFileName(userID)+".json")
}

func (s *fileAgentMemoryStore) sessionPath(sessionID string) string {
	return filepath.Join(s.baseDir, "sessions", safeFileName(sessionID)+".json")
}

func (s *fileAgentMemoryStore) conversationPath(sessionID string) string {
	return filepath.Join(s.baseDir, "conversations", safeFileName(sessionID)+".json")
}

func (s *fileAgentMemoryStore) sessionErrorPath(sessionID string) string {
	return filepath.Join(s.baseDir, "errors", safeFileName(sessionID)+".jsonl")
}

func (s *fileAgentMemoryStore) AppendSessionError(_ context.Context, record biz.SessionErrorRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record.SessionID = strings.TrimSpace(record.SessionID)
	if record.SessionID == "" {
		return errors.New("session id is required")
	}
	if record.Time.IsZero() {
		record.Time = time.Now()
	}
	path := s.sessionErrorPath(record.SessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure errors dir: %w", err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode session error: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open session error log %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("append session error log %s: %w", path, err)
	}
	if s.log != nil {
		s.log.Infof("session error recorded: %s stage=%s", path, record.Stage)
	}
	return nil
}

func (s *fileAgentMemoryStore) ListSessionErrors(_ context.Context, sessionID string, limit int) ([]biz.SessionErrorRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if limit <= 0 {
		limit = 8
	}
	path := s.sessionErrorPath(sessionID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session error log %s: %w", path, err)
	}
	var records []biz.SessionErrorRecord
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec biz.SessionErrorRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan session error log %s: %w", path, err)
	}
	if len(records) <= limit {
		return records, nil
	}
	return records[len(records)-limit:], nil
}

func (s *fileAgentMemoryStore) LoadConversation(_ context.Context, sessionID string) (*biz.SessionConversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	path := s.conversationPath(sessionID)
	memory, err := s.readConversationFile(path)
	if err != nil {
		return nil, err
	}
	if memory == nil {
		memory = &biz.SessionConversation{SessionID: sessionID}
	}
	memory.SessionID = sessionID
	return memory, nil
}

func (s *fileAgentMemoryStore) SaveConversation(_ context.Context, memory *biz.SessionConversation) error {
	if memory == nil {
		return errors.New("conversation is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	memory.SessionID = strings.TrimSpace(memory.SessionID)
	if memory.SessionID == "" {
		return errors.New("session id is required")
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = time.Now()
	}
	return s.writeJSON(s.conversationPath(memory.SessionID), memory)
}

func (s *fileAgentMemoryStore) readConversationFile(path string) (*biz.SessionConversation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read conversation %s: %w", path, err)
	}
	var memory biz.SessionConversation
	if err := json.Unmarshal(raw, &memory); err != nil {
		return nil, fmt.Errorf("decode conversation %s: %w", path, err)
	}
	return &memory, nil
}

func (s *fileAgentMemoryStore) normalizeUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return s.userID
	}
	return userID
}

func (s *fileAgentMemoryStore) readUserFile(path string) (*biz.UserAgentMemory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read user memory %s: %w", path, err)
	}
	var memory biz.UserAgentMemory
	if err := json.Unmarshal(raw, &memory); err != nil {
		return nil, fmt.Errorf("decode user memory %s: %w", path, err)
	}
	return &memory, nil
}

func (s *fileAgentMemoryStore) readSessionFile(path string) (*biz.SessionAgentMemory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session memory %s: %w", path, err)
	}
	var memory biz.SessionAgentMemory
	if err := json.Unmarshal(raw, &memory); err != nil {
		return nil, fmt.Errorf("decode session memory %s: %w", path, err)
	}
	return &memory, nil
}

func (s *fileAgentMemoryStore) writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure memory parent dir: %w", err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode memory %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write memory %s: %w", path, err)
	}
	if s.log != nil {
		s.log.Infof("agent memory saved: %s", path)
	}
	return nil
}

func safeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(value)
}
