package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"
	"law-enforcement-brain/pkg/utils"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

type ChatHandler struct {
	lawRepo   port.LawRepository
	llmClient port.LLMClient
}

func NewChatHandler(lawRepo port.LawRepository, llmClient port.LLMClient) *ChatHandler {
	return &ChatHandler{
		lawRepo:   lawRepo,
		llmClient: llmClient,
	}
}

type ChatRequest struct {
	Query     string    `json:"query" binding:"required"`
	ProjectID int64     `json:"project_id"`
	TopK      int       `json:"top_k"`
	History   []Message `json:"history"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message,omitempty"`
	Data    ChatData `json:"data,omitempty"`
}

type ChatData struct {
	Answer    string           `json:"answer"`
	Sources   []SourceDocument `json:"sources"`
	Timestamp int64            `json:"timestamp"`
}

type SourceDocument struct {
	ID       int64                  `json:"id"`
	LawName  string                 `json:"law_name"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

func (h *ChatHandler) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Failed to bind chat request", zap.Error(err))
		c.JSON(http.StatusBadRequest, ChatResponse{
			Success: false,
			Message: "Invalid request format: " + err.Error(),
		})
		return
	}

	if req.TopK == 0 {
		req.TopK = 5
	}

	ctx := c.Request.Context()

	queryVector, err := h.llmClient.CreateEmbedding(ctx, req.Query)
	if err != nil {
		logger.Log.Error("Failed to create embedding", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Success: false,
			Message: "Failed to create query embedding: " + err.Error(),
		})
		return
	}

	vectorStr := utils.FormatVector(queryVector)
	keywords := utils.ExtractKeywords(req.Query, 2)

	projectIDs := []int64{domain.DefaultProjectID}
	if req.ProjectID > 0 {
		projectIDs = []int64{req.ProjectID}
	}

	chunks, err := h.lawRepo.HybridSearch(ctx, projectIDs, vectorStr, keywords, req.TopK)
	if err != nil {
		logger.Log.Error("Failed to search knowledge base", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Success: false,
			Message: "Failed to search knowledge base: " + err.Error(),
		})
		return
	}

	var contextBuilder strings.Builder
	sources := make([]SourceDocument, 0, len(chunks))

	for i, chunk := range chunks {
		contextBuilder.WriteString(fmt.Sprintf("\n【参考资料 %d】\n", i+1))

		lawName := "未知文档"
		if name, ok := chunk.Metadata["law_name"].(string); ok && name != "" {
			lawName = name
			contextBuilder.WriteString(fmt.Sprintf("文档：%s\n", lawName))
		}

		if articleID, ok := chunk.Metadata["article_id"].(string); ok && articleID != "" {
			contextBuilder.WriteString(fmt.Sprintf("条款：%s\n", articleID))
		}

		contextBuilder.WriteString(fmt.Sprintf("内容：%s\n", chunk.Content))

		sources = append(sources, SourceDocument{
			ID:       chunk.ID,
			LawName:  lawName,
			Content:  chunk.Content,
			Metadata: chunk.Metadata,
		})
	}

	var messages []openai.ChatCompletionMessage

	systemPrompt := "你是一个专业的法律知识助手。请根据提供的参考资料回答用户的问题。如果参考资料中没有相关信息，请明确告知用户。回答要准确、专业、简洁。"
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	for _, msg := range req.History {
		role := openai.ChatMessageRoleUser
		if msg.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	userPrompt := fmt.Sprintf(`用户问题：%s

参考资料：
%s

请根据上述参考资料回答用户的问题。`, req.Query, contextBuilder.String())

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	answer, err := h.llmClient.ChatWithHistory(ctx, messages)
	if err != nil {
		logger.Log.Error("Failed to generate answer", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ChatResponse{
			Success: false,
			Message: "Failed to generate answer: " + err.Error(),
		})
		return
	}

	logger.Log.Info("Chat completed",
		zap.String("query", req.Query),
		zap.Int("sources_count", len(sources)),
		zap.Int("answer_length", len(answer)))

	c.JSON(http.StatusOK, ChatResponse{
		Success: true,
		Data: ChatData{
			Answer:    answer,
			Sources:   sources,
			Timestamp: time.Now().Unix(),
		},
	})
}
