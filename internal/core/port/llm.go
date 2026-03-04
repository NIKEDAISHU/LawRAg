package port

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
)

// CaseReviewLLM 案件评审专用 LLM 接口
// 负责从案件内容中提取关键信息和验证法律引用
type CaseReviewLLM interface {
	// ExtractInfo 从案件内容中提取关键信息（违法事实、引用法条、关键事实）
	ExtractInfo(ctx context.Context, caseContent string) (string, error)
	// ValidateLaws 验证法律引用的准确性和合法性
	ValidateLaws(ctx context.Context, facts string, citedLaws string, retrievedLaws string) (string, error)
}

// EmbeddingLLM 向量生成专用接口
type EmbeddingLLM interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}

// ChatLLM 对话专用接口
type ChatLLM interface {
	Chat(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
	ChatWithHistory(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error)
	ChatStream(ctx context.Context, systemPrompt string, userPrompt string, callback func(content string) error) error
	ChatWithHistoryStream(ctx context.Context, messages []openai.ChatCompletionMessage, callback func(content string) error) error
}

// QueryRewriteLLM 查询重写专用接口
type QueryRewriteLLM interface {
	RewriteQuery(ctx context.Context, ocrText string, rule string) (*QueryRewriteResult, error)
}

// LLMClient
type LLMClient interface {
	CaseReviewLLM
	EmbeddingLLM
	ChatLLM
	QueryRewriteLLM
}

// QueryRewriteResult 查询重写结果
type QueryRewriteResult struct {
	Keywords         []string // 提取的关键词，用于全文检索
	NormalizedQuery  string   // 规范化描述，用于向量检索
	LawCategory      string   // 法律类别过滤条件（可选）
	CorrectedOCRText string   // 修正后的 OCR 文本（可选）
}
