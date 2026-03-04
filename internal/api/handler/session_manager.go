package handler

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sashabaranov/go-openai"
)

// SessionManager 管理 LLM 对话会话，支持批量请求共享上下文
type SessionManager struct {
	sessions            map[string]*Session
	mu                  sync.RWMutex
	maxSize             int           // 每个会话最多包含的请求数
	maxTokens           int           // 每个会话最大的 token 数限制（粗略估计）
	ttl                 time.Duration // 会话过期时间
	done                chan struct{} // 用于优雅关闭清理协程
	maxSessions         int32         // 最大会话数限制
	cleanupInterval     time.Duration // 清理间隔
	currentSessionCount int32         // 当前会话数（原子操作）
}

// Session 单个会话
type Session struct {
	ID                  string
	Messages            []openai.ChatCompletionMessage
	RequestCount        int
	CreatedAt           time.Time
	LastUsedAt          time.Time
	TotalPromptLen      int          // 缓存 prompt 总长度，避免重复计算
	EstimatedTokenCount int          // 估算的 token 数（1 token ≈ 4 字符）
	mu                  sync.RWMutex // 会话级别的锁，保护并发访问
	lastRequestTime     time.Time    // 上次请求时间，用于简单的速率限制
}

// NewSessionManager 创建会话管理器
func NewSessionManager(maxSize int, ttl time.Duration) *SessionManager {
	sm := &SessionManager{
		sessions:            make(map[string]*Session),
		maxSize:             maxSize,
		maxTokens:           8000, // 默认最大 token 限制
		ttl:                 ttl,
		done:                make(chan struct{}),
		maxSessions:         1000, // 最大会话数
		cleanupInterval:     5 * time.Minute,
		currentSessionCount: 0,
	}

	// 启动清理协程
	go sm.cleanupExpiredSessions()

	return sm
}

// SetMaxTokens 设置最大 token 限制
func (sm *SessionManager) SetMaxTokens(maxTokens int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxTokens = maxTokens
}

// SetMaxSessions 设置最大会话数
func (sm *SessionManager) SetMaxSessions(maxSessions int32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxSessions = maxSessions
}

// GetOrCreateSession 获取或创建会话
func (sm *SessionManager) GetOrCreateSession(sessionID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 检查会话数是否超过限制
	if atomic.LoadInt32(&sm.currentSessionCount) >= sm.maxSessions {
		// 清理最旧的会话
		sm.evictOldestSession()
	}

	session, exists := sm.sessions[sessionID]
	if !exists {
		// 估算 token：粗略按 4 字符 = 1 token
		session = &Session{
			ID:                  sessionID,
			Messages:            make([]openai.ChatCompletionMessage, 0, 12), // 预分配容量：system + 5*(user+assistant)
			CreatedAt:           time.Now(),
			LastUsedAt:          time.Now(),
			EstimatedTokenCount: 0,
			lastRequestTime:     time.Now(),
		}
		sm.sessions[sessionID] = session
		atomic.AddInt32(&sm.currentSessionCount, 1)
	}

	// 在锁内更新 LastUsedAt
	session.mu.Lock()
	session.LastUsedAt = time.Now()
	session.mu.Unlock()

	return session
}

// AddMessage 添加消息到会话
func (sm *SessionManager) AddMessage(sessionID string, role string, content string) {
	sm.mu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return
	}

	// 使用会话级别的锁
	session.mu.Lock()
	defer session.mu.Unlock()

	// 简单的速率限制：如果请求过于频繁（< 100ms），可以选择记录日志
	if time.Since(session.lastRequestTime) < 100*time.Millisecond {
		// 请求过于频繁，但这里我们选择继续
	}
	session.lastRequestTime = time.Now()

	session.Messages = append(session.Messages, openai.ChatCompletionMessage{
		Role:    role,
		Content: content,
	})

	// 只保留最近4轮对话（8条消息 + system prompt = 最多9条）
	// 保留 system prompt，从第2条消息开始截断
	maxMessages := 9 // 1条 system + 4对对话
	if len(session.Messages) > maxMessages {
		// 保留第1条 system prompt，截断后面的
		session.Messages = append(session.Messages[:1], session.Messages[len(session.Messages)-maxMessages+1:]...)
		// 重新计算 TotalPromptLen
		session.TotalPromptLen = 0
		for _, msg := range session.Messages {
			session.TotalPromptLen += len(msg.Content)
		}
		session.EstimatedTokenCount = session.TotalPromptLen / 4
	} else {
		session.TotalPromptLen += len(content)
		// 估算 token 数（粗略：4 字符 = 1 token）
		session.EstimatedTokenCount += len(content) / 4
	}

	session.RequestCount++
	session.LastUsedAt = time.Now()

	// 检查是否需要重置
	needReset := session.RequestCount >= sm.maxSize || session.EstimatedTokenCount >= sm.maxTokens

	// 如果达到最大请求数或 token 限制，同步重置会话
	// 注意：这里使用同步调用以避免竞态条件，resetSession 操作很快不会造成明显阻塞
	if needReset {
		sm.resetSession(sessionID)
	}
}

// GetMessages 获取会话的所有消息（返回副本，避免并发问题）
func (sm *SessionManager) GetMessages(sessionID string) []openai.ChatCompletionMessage {
	sm.mu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return nil
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	// 返回副本，避免外部修改
	messages := make([]openai.ChatCompletionMessage, len(session.Messages))
	copy(messages, session.Messages)
	return messages
}

// resetSession 重置会话（保留 system prompt）
func (sm *SessionManager) resetSession(sessionID string) {
	sm.mu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// 只保留第一条 system message
	if len(session.Messages) > 0 && session.Messages[0].Role == openai.ChatMessageRoleSystem {
		systemLen := len(session.Messages[0].Content)
		session.Messages = session.Messages[:1]
		session.TotalPromptLen = systemLen
		session.EstimatedTokenCount = systemLen / 4
	} else {
		session.Messages = make([]openai.ChatCompletionMessage, 0, 12)
		session.TotalPromptLen = 0
		session.EstimatedTokenCount = 0
	}
	session.RequestCount = 0
}

// evictOldestSession 淘汰最旧的会话
func (sm *SessionManager) evictOldestSession() {
	var oldestID string
	var oldestTime time.Time

	for id, session := range sm.sessions {
		session.mu.RLock()
		lastUsed := session.LastUsedAt
		session.mu.RUnlock()

		if oldestID == "" || lastUsed.Before(oldestTime) {
			oldestID = id
			oldestTime = lastUsed
		}
	}

	if oldestID != "" {
		delete(sm.sessions, oldestID)
		atomic.AddInt32(&sm.currentSessionCount, -1)
	}
}

// GetSessionInfo 获取会话信息（用于日志记录）
func (sm *SessionManager) GetSessionInfo(sessionID string) (requestCount int, messageCount int, totalPromptLen int, estimatedTokens int) {
	sm.mu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return 0, 0, 0, 0
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.RequestCount, len(session.Messages), session.TotalPromptLen, session.EstimatedTokenCount
}

// GetStats 获取会话管理器统计信息
func (sm *SessionManager) GetStats() (sessionCount int, totalMessages int, totalTokens int64) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessionCount = len(sm.sessions)
	for _, session := range sm.sessions {
		session.mu.RLock()
		totalMessages += len(session.Messages)
		totalTokens += int64(session.EstimatedTokenCount)
		session.mu.RUnlock()
	}

	return sessionCount, totalMessages, totalTokens
}

// Close 优雅关闭会话管理器
func (sm *SessionManager) Close() {
	close(sm.done)
}

// cleanupExpiredSessions 定期清理过期会话
func (sm *SessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(sm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanup()
		case <-sm.done:
			return
		}
	}
}

// cleanup 清理过期会话
func (sm *SessionManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	deletedCount := 0

	for id, session := range sm.sessions {
		session.mu.RLock()
		lastUsed := session.LastUsedAt
		session.mu.RUnlock()

		if now.Sub(lastUsed) > sm.ttl {
			delete(sm.sessions, id)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		atomic.AddInt32(&sm.currentSessionCount, -int32(deletedCount))
	}
}
