package handler

import (
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
)

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager(5, 10*time.Minute)
	if sm == nil {
		t.Fatal("NewSessionManager returned nil")
	}
	if sm.maxSize != 5 {
		t.Errorf("expected maxSize 5, got %d", sm.maxSize)
	}
	if sm.ttl != 10*time.Minute {
		t.Errorf("expected ttl 10m, got %v", sm.ttl)
	}
}

func TestSessionManager_GetOrCreateSession(t *testing.T) {
	sm := NewSessionManager(5, 10*time.Minute)
	defer sm.Close()

	// 创建新会话
	session := sm.GetOrCreateSession("test-session")
	if session == nil {
		t.Fatal("GetOrCreateSession returned nil")
	}
	if session.ID != "test-session" {
		t.Errorf("expected session ID 'test-session', got '%s'", session.ID)
	}
	if session.RequestCount != 0 {
		t.Errorf("expected RequestCount 0, got %d", session.RequestCount)
	}

	// 获取已存在的会话
	session2 := sm.GetOrCreateSession("test-session")
	if session2 != session {
		t.Error("GetOrCreateSession should return the same session")
	}
}

func TestSessionManager_AddMessage(t *testing.T) {
	sm := NewSessionManager(5, 10*time.Minute)
	defer sm.Close()

	sm.GetOrCreateSession("test-session")

	// 添加用户消息
	sm.AddMessage("test-session", openai.ChatMessageRoleUser, "Hello")
	sm.AddMessage("test-session", openai.ChatMessageRoleAssistant, "Hi there")

	messages := sm.GetMessages("test-session")
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("expected first message role to be user, got %s", messages[0].Role)
	}
	if messages[0].Content != "Hello" {
		t.Errorf("expected first message content to be 'Hello', got '%s'", messages[0].Content)
	}
	if messages[1].Role != openai.ChatMessageRoleAssistant {
		t.Errorf("expected second message role to be assistant, got %s", messages[1].Role)
	}
}

func TestSessionManager_GetMessages(t *testing.T) {
	sm := NewSessionManager(5, 10*time.Minute)
	defer sm.Close()

	// 获取不存在的会话
	messages := sm.GetMessages("non-existent")
	if messages != nil {
		t.Error("expected nil for non-existent session")
	}

	// 创建会话并添加消息
	sm.GetOrCreateSession("test-session")
	sm.AddMessage("test-session", openai.ChatMessageRoleUser, "Test message")

	// 获取消息
	messages = sm.GetMessages("test-session")
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}

	// 验证返回的是副本
	messages[0].Content = "Modified"
	messages2 := sm.GetMessages("test-session")
	if messages2[0].Content == "Modified" {
		t.Error("GetMessages should return a copy, not the original")
	}
}

func TestSessionManager_resetSession(t *testing.T) {
	sm := NewSessionManager(5, 10*time.Minute)
	defer sm.Close()

	sm.GetOrCreateSession("test-session")
	sm.AddMessage("test-session", openai.ChatMessageRoleSystem, "System prompt")
	sm.AddMessage("test-session", openai.ChatMessageRoleUser, "User message")
	sm.AddMessage("test-session", openai.ChatMessageRoleAssistant, "Assistant message")

	// 验证消息已添加
	messages := sm.GetMessages("test-session")
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages before reset, got %d", len(messages))
	}

	// 重置会话
	sm.resetSession("test-session")

	// 验证只保留了 system prompt
	messages = sm.GetMessages("test-session")
	if len(messages) != 1 {
		t.Errorf("expected 1 message after reset, got %d", len(messages))
	}
	if messages[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("expected system message to remain, got %s", messages[0].Role)
	}
}

func TestSessionManager_GetSessionInfo(t *testing.T) {
	sm := NewSessionManager(5, 10*time.Minute)
	defer sm.Close()

	// 获取不存在的会话信息
	reqCount, msgCount, promptLen, tokens := sm.GetSessionInfo("non-existent")
	if reqCount != 0 || msgCount != 0 || promptLen != 0 || tokens != 0 {
		t.Error("expected all zeros for non-existent session")
	}

	// 创建会话并添加消息
	sm.GetOrCreateSession("test-session")
	sm.AddMessage("test-session", openai.ChatMessageRoleUser, "Hello world!")

	reqCount, msgCount, promptLen, tokens = sm.GetSessionInfo("test-session")
	if reqCount != 1 {
		t.Errorf("expected request count 1, got %d", reqCount)
	}
	if msgCount != 1 {
		t.Errorf("expected message count 1, got %d", msgCount)
	}
	if promptLen != 12 { // "Hello world!" = 12 chars
		t.Errorf("expected prompt len 12, got %d", promptLen)
	}
	if tokens != 3 { // 12 / 4 = 3
		t.Errorf("expected tokens 3, got %d", tokens)
	}
}

func TestSessionManager_MaxSizeReset(t *testing.T) {
	sm := NewSessionManager(2, 10*time.Minute)
	defer sm.Close()

	sm.GetOrCreateSession("test-session")

	// 添加两条消息
	sm.AddMessage("test-session", openai.ChatMessageRoleUser, "Message 1")
	sm.AddMessage("test-session", openai.ChatMessageRoleAssistant, "Response 1")

	// 验证消息已添加
	messages := sm.GetMessages("test-session")
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// 添加第三条消息，触发重置
	sm.AddMessage("test-session", openai.ChatMessageRoleUser, "Message 2")

	// 验证会话已重置，只保留 system prompt
	messages = sm.GetMessages("test-session")
	// 注意：由于我们添加了 system prompt 在重置时被保留，所以可能是 1 条
	if len(messages) > 2 {
		t.Errorf("session should be reset after max size, got %d messages", len(messages))
	}
}

func TestSessionManager_concurrentAccess(t *testing.T) {
	sm := NewSessionManager(100, 10*time.Minute)
	defer sm.Close()

	done := make(chan bool)
	numGoroutines := 10
	numMessages := 100

	// 并发添加消息
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numMessages; j++ {
				sessionID := "session-"
				if j%2 == 0 {
					sessionID += "even"
				} else {
					sessionID += "odd"
				}
				sm.GetOrCreateSession(sessionID)
				sm.AddMessage(sessionID, openai.ChatMessageRoleUser, "Message")
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 验证两个会话都存在且有正确的消息数
	messagesEven := sm.GetMessages("session-even")
	messagesOdd := sm.GetMessages("session-odd")

	expectedCount := numGoroutines * (numMessages / 2)
	if len(messagesEven) != expectedCount {
		t.Errorf("expected %d messages in even session, got %d", expectedCount, len(messagesEven))
	}
	if len(messagesOdd) != expectedCount {
		t.Errorf("expected %d messages in odd session, got %d", expectedCount, len(messagesOdd))
	}
}
