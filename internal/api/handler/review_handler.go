package handler

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/internal/core/service"
	"law-enforcement-brain/pkg/logger"
	"law-enforcement-brain/pkg/utils"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

type ReviewHandler struct {
	reviewService  *service.ReviewService
	lawRepo        port.LawRepository
	llmClient      port.LLMClient
	sessionManager *SessionManager
	closeOnce      sync.Once
}

func NewReviewHandler(reviewService *service.ReviewService, lawRepo port.LawRepository, llmClient port.LLMClient) *ReviewHandler {
	return &ReviewHandler{
		reviewService:  reviewService,
		lawRepo:        lawRepo,
		llmClient:      llmClient,
		sessionManager: NewSessionManager(5, 30*time.Minute), // 每5个请求一个会话，30分钟过期
	}
}

// Close 释放 ReviewHandler 资源
func (h *ReviewHandler) Close() {
	h.closeOnce.Do(func() {
		if h.sessionManager != nil {
			h.sessionManager.Close()
		}
	})
}

func (h *ReviewHandler) ReviewCase(c *gin.Context) {
	var req domain.ReviewRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Failed to bind request", zap.Error(err))
		c.JSON(http.StatusBadRequest, domain.ReviewResponse{
			Code: http.StatusBadRequest,
			Msg:  "Invalid request format: " + err.Error(),
		})
		return
	}

	result, err := h.reviewService.ReviewCase(c.Request.Context(), req)
	if err != nil {
		logger.Log.Error("Failed to review case",
			zap.String("request_id", req.RequestID),
			zap.Error(err))

		c.JSON(http.StatusInternalServerError, domain.ReviewResponse{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to review case: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, domain.ReviewResponse{
		Code: http.StatusOK,
		Data: *result,
	})
}

func (h *ReviewHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "law-enforcement-brain",
	})
}

// Dify 兼容的请求结构
type DifyChatRequest struct {
	Inputs struct {
		EventNarrative string `json:"event_narrative"` // 文书内容（OCR文本）
	} `json:"inputs"`
	Query          string `json:"query"` // 评审规则/任务
	User           string `json:"user"`  // 规则编号
	ResponseMode   string `json:"response_mode"`
	ConversationID string `json:"conversation_id"`
}

// Dify 兼容的响应结构
type DifyChatResponse struct {
	Event          string `json:"event"`
	TaskID         string `json:"task_id"`
	ID             string `json:"id"`
	Mode           string `json:"mode"`
	Answer         string `json:"answer"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	CreatedAt      int64  `json:"created_at"`
	Metadata       struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		RetrieverResources []RetrieverResource `json:"retriever_resources"`
	} `json:"metadata"`
}

type RetrieverResource struct {
	Position     int     `json:"position"`
	DatasetID    string  `json:"dataset_id"`
	DatasetName  string  `json:"dataset_name"`
	DocumentID   string  `json:"document_id"`
	DocumentName string  `json:"document_name"`
	SegmentID    string  `json:"segment_id"`
	Score        float64 `json:"score"`
	Content      string  `json:"content"`
}

// DifyChatCompletion Dify 兼容的评审接口
func (h *ReviewHandler) DifyChatCompletion(c *gin.Context) {
	var req DifyChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Failed to bind Dify request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// event_narrative: 文书内容（OCR文本）
	ocrText := req.Inputs.EventNarrative
	if ocrText == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "event_narrative is required",
		})
		return
	}

	// query: 评审规则/任务
	rule := req.Query
	if rule == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query (review rule) is required",
		})
		return
	}

	// user: 规则编号
	ruleID := req.User

	// 直接使用 OCR 文本进行检索，无需查询重写
	normalizedQuery := ocrText
	keywords := []string{ocrText}

	// Step 1: 生成查询向量
	// 如果 embedding 服务不可用，降级为纯关键词检索
	var chunks []domain.LawChunk
	queryVector, err := h.llmClient.CreateEmbedding(ctx, normalizedQuery)
	if err != nil {
		logger.Log.Warn("Failed to create query embedding, fallback to keyword-only search", zap.Error(err))
		// 降级：使用零向量进行检索（实际只使用关键词检索）
		vectorStr := "[" + strings.Repeat("0,", 1023) + "0]"
		defaultProjectIDs := []int64{domain.DefaultProjectID}
		chunks, err = h.lawRepo.HybridSearch(ctx, defaultProjectIDs, vectorStr, keywords, 5)
		if err != nil {
			logger.Log.Error("Failed to search", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to search knowledge base",
			})
			return
		}
	} else {
		// 使用优化的向量格式转换
		vectorStr := utils.FormatVectorWithoutCache(queryVector)

		logger.Log.Info("Vector conversion",
			zap.Int("vector_length", len(queryVector)),
			zap.Int("vector_str_length", len(vectorStr)),
			zap.String("vector_preview", vectorStr[:min(100, len(vectorStr))]))

		// Step 3: 混合检索（向量 + 全文）使用提取的关键词
		defaultProjectIDs := []int64{domain.DefaultProjectID}
		chunks, err = h.lawRepo.HybridSearch(ctx, defaultProjectIDs, vectorStr, keywords, 5)
		if err != nil {
			logger.Log.Error("Failed to search", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to search knowledge base",
			})
			return
		}
	}

	// 构建上下文
	context := ""
	retrieverResources := make([]RetrieverResource, 0, len(chunks))
	for i, chunk := range chunks {
		context += fmt.Sprintf("\n【参考资料 %d】\n%s\n", i+1, chunk.Content)

		// 构建检索资源
		docName := "未知文档"
		if lawName, ok := chunk.Metadata["law_name"].(string); ok && lawName != "" {
			docName = lawName
		}

		retrieverResources = append(retrieverResources, RetrieverResource{
			Position:     i + 1,
			DatasetID:    "law_kb",
			DatasetName:  "法律法规知识库",
			DocumentID:   fmt.Sprintf("doc_%d", chunk.ID),
			DocumentName: docName,
			SegmentID:    fmt.Sprintf("seg_%d", chunk.ID),
			Score:        0.95, // 简化处理，实际应该从检索结果中获取
			Content:      chunk.Content,
		})
	}

	// 每次请求都是新会话，不累积历史消息
	var messages []openai.ChatCompletionMessage

	// 添加 system prompt
	systemPrompt := fmt.Sprintf("你是一个专业的法律执法案件评审助手（规则编号：%s）。请根据提供的参考资料和案件文书，严格按照评审规则进行评审分析。", ruleID)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})

	// 添加当前请求的 user prompt
	userPrompt := fmt.Sprintf(`评审规则：
%s

案件文书（OCR识别）：
%s

参考法律法规：
%s

请严格按照上述评审规则，结合案件文书和参考法律法规，进行评审分析。`, rule, ocrText, context)

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	// 调用 LLM 生成评审结果（单轮对话）
	answer, err := h.llmClient.ChatWithHistory(ctx, messages)
	if err != nil {
		logger.Log.Error("Failed to generate answer", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate answer",
		})
		return
	}

	// 记录日志
	promptLen := len(userPrompt) + len(systemPrompt)
	logger.Log.Info("Chat completed",
		zap.String("rule_id", ruleID),
		zap.Int("prompt_len", promptLen),
		zap.Int("answer_length", len(answer)))

	// 构建 Dify 兼容的响应
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	conversationID := req.ConversationID
	if conversationID == "" {
		conversationID = fmt.Sprintf("conv_%d", time.Now().UnixNano())
	}

	response := DifyChatResponse{
		Event:          "message",
		TaskID:         taskID,
		ID:             messageID,
		Mode:           "chat",
		Answer:         answer,
		ConversationID: conversationID,
		MessageID:      messageID,
		CreatedAt:      time.Now().Unix(),
	}

	// 设置 token 使用情况
	response.Metadata.Usage.PromptTokens = promptLen / 4
	response.Metadata.Usage.CompletionTokens = len(answer) / 4
	response.Metadata.Usage.TotalTokens = response.Metadata.Usage.PromptTokens + response.Metadata.Usage.CompletionTokens
	response.Metadata.RetrieverResources = retrieverResources

	c.JSON(http.StatusOK, response)
}
