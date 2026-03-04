package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/config"

	openai "github.com/sashabaranov/go-openai"
)

// QwenClient 阿里百炼 Qwen 模型客户端
type QwenClient struct {
	httpClient      *http.Client
	baseURL         string
	apiKey          string
	model           string
	embeddingURL    string
	embeddingKey    string
	embeddingModel  string
	localOCRBaseURL string
	localOCRModel   string
}

func NewQwenClient(cfg *config.LLMConfig) port.LLMClient {
	baseURL, apiKey, model := cfg.GetActiveLLMConfig()
	return &QwenClient{
		httpClient:      &http.Client{Timeout: 300 * time.Second}, // 5 分钟超时
		baseURL:         baseURL,
		apiKey:          apiKey,
		model:           model,
		embeddingURL:    cfg.OllamaBaseURL,
		embeddingKey:    cfg.APIKey,
		embeddingModel:  cfg.EmbeddingModel,
		localOCRBaseURL: cfg.LocalOCRBaseURL,
		localOCRModel:   cfg.LocalOCRModel,
	}
}

type qwenRequest struct {
	Model       string                   `json:"model"`
	MaxTokens   int64                    `json:"max_tokens,omitempty"`
	System      string                   `json:"system,omitempty"`
	Messages    []qwenMessageContent     `json:"messages"`
	Stream      bool                     `json:"stream,omitempty"`
	Temperature float64                  `json:"temperature,omitempty"`
	// 禁用思考模式，提升响应速度
	Thinking *qwenThinking `json:"thinking,omitempty"`
}

type qwenThinking struct {
	Type string `json:"type"`
}

type qwenMessageContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      qwenMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error qwenError `json:"error"`
}

type qwenMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func (c *QwenClient) qwenRequest(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	messages := []qwenMessageContent{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reqBody := qwenRequest{
		Model:       c.model,
		MaxTokens:   4096,
		System:      systemPrompt,
		Messages:    messages,
		Temperature: 0.7,
		Thinking:   &qwenThinking{Type: "disabled"},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error: %s, body: %s", resp.Status, string(body))
	}

	var qwenResp qwenResponse
	if err := json.Unmarshal(body, &qwenResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if qwenResp.Error.Message != "" {
		return "", fmt.Errorf("API error: %s", qwenResp.Error.Message)
	}

	if len(qwenResp.Choices) == 0 {
		return "", fmt.Errorf("no response from Qwen API")
	}

	return qwenResp.Choices[0].Message.Content, nil
}

// Chat 简单的聊天接口
func (c *QwenClient) Chat(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	return c.qwenRequest(ctx, systemPrompt, userPrompt)
}

// ChatWithHistory 支持多轮对话的聊天方法
func (c *QwenClient) ChatWithHistory(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("empty messages")
	}

	var systemPrompt string
	var qwenMessages []qwenMessageContent

	for i, msg := range messages {
		if msg.Role == openai.ChatMessageRoleSystem && i == 0 {
			systemPrompt = msg.Content
		} else {
			qwenMessages = append(qwenMessages, qwenMessageContent{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// 如果没有 system prompt，使用默认
	if systemPrompt == "" {
		systemPrompt = "你是一个专业的法律助手。"
	}

	reqBody := qwenRequest{
		Model:       c.model,
		MaxTokens:   4096,
		System:      systemPrompt,
		Messages:    qwenMessages,
		Temperature: 0.7,
		Thinking:   &qwenThinking{Type: "disabled"}, // 禁用思考模式，提升响应速度
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error: %s, body: %s", resp.Status, string(body))
	}

	var qwenResp qwenResponse
	if err := json.Unmarshal(body, &qwenResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if qwenResp.Error.Message != "" {
		return "", fmt.Errorf("API error: %s", qwenResp.Error.Message)
	}

	if len(qwenResp.Choices) == 0 {
		return "", fmt.Errorf("no response from Qwen API")
	}

	return qwenResp.Choices[0].Message.Content, nil
}

// ExtractInfo 提取案件信息
func (c *QwenClient) ExtractInfo(ctx context.Context, caseContent string) (string, error) {
	prompt := fmt.Sprintf(`请从以下案件内容中提取关键信息，包括：
1. 当事人信息（姓名、身份证号、联系方式等）
2. 涉案时间
3. 涉案地点
4. 案件类型
5. 主要事实经过

案件内容：
%s

请以JSON格式输出关键信息。`, caseContent)

	return c.qwenRequest(ctx, "你是一个专业的法律案件信息提取助手。", prompt)
}

// ValidateLaws 验证法律引用
func (c *QwenClient) ValidateLaws(ctx context.Context, facts string, citedLaws string, retrievedLaws string) (string, error) {
	prompt := fmt.Sprintf(`请验证以下案件事实与引用的法律条文是否匹配：

案件事实：
%s

引用的法律条文：
%s

检索到的相关法律条文：
%s

请分析引用是否准确、适当。`, facts, citedLaws, retrievedLaws)

	return c.qwenRequest(ctx, "你是一个专业的法律审核专家。", prompt)
}

// CreateEmbedding 创建向量嵌入
func (c *QwenClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model":  c.embeddingModel,
		"prompt": text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.embeddingURL+"/api/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API error: %s, body: %s", resp.Status, string(body))
	}

	var embResp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return embResp.Embedding, nil
}

// RewriteQuery 查询重写（当前未使用）
func (c *QwenClient) RewriteQuery(ctx context.Context, ocrText string, rule string) (*port.QueryRewriteResult, error) {
	prompt := fmt.Sprintf(`你是一个法律检索助手。用户提供了 OCR 文本和评审规则，请帮助提取检索所需的信息。

OCR 文本：%s

评审规则：%s

请提取3-5个核心搜索关键词，必须严格按照以下JSON格式输出：
{"keywords": ["关键词1", "关键词2"], "normalized_query": "规范描述", "law_category": ""}`, ocrText, rule)

	content, err := c.qwenRequest(ctx, "你是一个法律检索助手，必须严格按照JSON格式输出。", prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to rewrite query: %w", err)
	}

	// 去除 markdown 代码块
	content = extractJSON(content)

	var result port.QueryRewriteResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse rewrite result: %w", err)
	}

	return &result, nil
}

// ChatStream 流式聊天
func (c *QwenClient) ChatStream(ctx context.Context, systemPrompt string, userPrompt string, callback func(content string) error) error {
	messages := []qwenMessageContent{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reqBody := qwenRequest{
		Model:       c.model,
		MaxTokens:   4096,
		System:      systemPrompt,
		Messages:    messages,
		Stream:      true,
		Temperature: 0.7,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s, body: %s", resp.Status, string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk map[string]interface{}
		if err := decoder.Decode(&chunk); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("decode error: %w", err)
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		delta, ok := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := delta["content"].(string)
		if !ok || content == "" {
			continue
		}

		if err := callback(content); err != nil {
			return err
		}
	}

	return nil
}

// ChatWithHistoryStream 支持多轮对话的流式聊天
func (c *QwenClient) ChatWithHistoryStream(ctx context.Context, messages []openai.ChatCompletionMessage, callback func(content string) error) error {
	var systemPrompt string
	var qwenMessages []qwenMessageContent

	for i, msg := range messages {
		if msg.Role == openai.ChatMessageRoleSystem && i == 0 {
			systemPrompt = msg.Content
		} else {
			qwenMessages = append(qwenMessages, qwenMessageContent{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	if systemPrompt == "" {
		systemPrompt = "你是一个专业的法律助手。"
	}

	reqBody := qwenRequest{
		Model:       c.model,
		MaxTokens:   4096,
		System:      systemPrompt,
		Messages:    qwenMessages,
		Stream:      true,
		Temperature: 0.7,
		Thinking:   &qwenThinking{Type: "disabled"},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s, body: %s", resp.Status, string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk map[string]interface{}
		if err := decoder.Decode(&chunk); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("decode error: %w", err)
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		delta, ok := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := delta["content"].(string)
		if !ok || content == "" {
			continue
		}

		if err := callback(content); err != nil {
			return err
		}
	}

	return nil
}
