package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/pkg/logger"
	"law-enforcement-brain/pkg/utils"
)

// Reranker 重排器
type Reranker struct {
	cfg    *Config
	client *http.Client
}

// Config Rerank 配置
type Config struct {
	BaseURL string // Ollama 地址，如 http://localhost:11434
	APIKey  string // 可选，某些服务需要
	Model   string // 模型名称，如 qllama/bge-reranker-v2-m3
}

// RerankRequest 重排请求
type RerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
	Model     string   `json:"model,omitempty"`
}

// RerankResponse 重排响应
type RerankResponse struct {
	ID      string       `json:"id"`
	Results []RerankItem `json:"results"`
}

// RerankItem 重排结果项
type RerankItem struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// NewReranker 创建重排器
func NewReranker(cfg *Config) *Reranker {
	return &Reranker{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Rerank 重排 chunks
func (r *Reranker) Rerank(ctx context.Context, query string, chunks []domain.LawChunk, topK int) ([]domain.LawChunk, error) {
	if r.cfg == nil || r.cfg.BaseURL == "" {
		// 未配置 rerank，直接返回原结果
		return chunks, nil
	}

	if len(chunks) == 0 {
		return chunks, nil
	}

	if topK <= 0 {
		topK = len(chunks)
	}

	// 提取文档内容
	docs := make([]string, len(chunks))
	for i, chunk := range chunks {
		docs[i] = chunk.Content
	}

	// 调用 rerank API
	results, err := r.callRerankAPI(ctx, query, docs, topK)
	if err != nil {
		logger.Log.Warn("Rerank failed, fallback to original results", zap.Error(err))
		return chunks, nil
	}

	// 根据重排结果重新组装 chunks
	reranked := make([]domain.LawChunk, 0, len(results))
	for _, item := range results {
		if item.Index < len(chunks) {
			chunk := chunks[item.Index]
			// 存储重排后的分数
			if chunk.Metadata == nil {
				chunk.Metadata = make(map[string]interface{})
			}
			chunk.Metadata["rerank_score"] = item.RelevanceScore
			reranked = append(reranked, chunk)
		}
	}

	return reranked, nil
}

// RerankWithQA 使用 QA 内容重排
func (r *Reranker) RerankWithQA(ctx context.Context, query string, chunks []domain.LawChunk, topK int) ([]domain.LawChunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}

	// 优先使用 QA 内容，没有则使用原文
	docs := make([]string, len(chunks))
	for i, chunk := range chunks {
		if chunk.QAContent != "" {
			// 使用 QA 内容（用户提问更可能是针对问题的）
			docs[i] = chunk.QAContent
		} else {
			docs[i] = chunk.Content
		}
	}

	return r.Rerank(ctx, query, chunks, topK)
}

func (r *Reranker) callRerankAPI(ctx context.Context, query string, docs []string, topK int) ([]RerankItem, error) {
	reqBody := RerankRequest{
		Query:     query,
		Documents: docs,
		TopN:      topK,
		Model:     r.cfg.Model,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// Ollama rerank API 格式
	url := fmt.Sprintf("%s/api/rerank", r.cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if r.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", r.cfg.APIKey))
	}

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result RerankResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w", err)
	}

	return result.Results, nil
}

// SimpleRerank 简单重排（使用关键词匹配权重）
func SimpleRerank(ctx context.Context, query string, chunks []domain.LawChunk) []domain.LawChunk {
	// 提取关键词
	keywords := utils.ExtractKeywords(query, 2)

	// 计算每个 chunk 的相关性分数
	type scoredChunk struct {
		chunk domain.LawChunk
		score float64
	}

	scored := make([]scoredChunk, len(chunks))
	for i, chunk := range chunks {
		score := calculateRelevanceScore(chunk.Content, keywords)
		if chunk.QAContent != "" {
			// QA 内容权重更高
			qaScore := calculateRelevanceScore(chunk.QAContent, keywords)
			score = score*0.7 + qaScore*0.3
		}
		scored[i] = scoredChunk{chunk: chunk, score: score}
	}

	// 按分数排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]domain.LawChunk, len(chunks))
	for i, sc := range scored {
		result[i] = sc.chunk
	}

	return result
}

// calculateRelevanceScore 计算相关性分数
func calculateRelevanceScore(content string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	score := 0.0
	for _, kw := range keywords {
		if len(kw) > 1 {
			// 关键词出现次数
			count := strings.Count(content, kw)
			score += float64(count) * 10
			// 首次出现加分
			if idx := strings.Index(content, kw); idx != -1 && idx < 100 {
				score += (100 - float64(idx)) / 10
			}
		}
	}

	// 归一化
	maxScore := float64(len(keywords)) * 15
	if maxScore == 0 {
		return 0
	}
	if score > maxScore {
		score = maxScore
	}
	return score / maxScore
}
