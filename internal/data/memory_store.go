package data

import (
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
	if dataConf != nil && dataConf.GetAgentMemory() != nil {
		if configured := strings.TrimSpace(dataConf.GetAgentMemory().GetDir()); configured != "" {
			dir = configured
		}
		if configuredUser := strings.TrimSpace(dataConf.GetAgentMemory().GetUserId()); configuredUser != "" {
			userID = configuredUser
		}
	}
	return biz.AgentMemoryConfig{Dir: dir, UserID: userID}
}

func (s *fileAgentMemoryStore) ensureLayout() error {
	for _, sub := range []string{"users", "sessions", "conversations"} {
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
