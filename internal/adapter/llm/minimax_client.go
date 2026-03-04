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

// MiniMaxClient MiniMax AI 客户端 (Anthropic 兼容 API)
type MiniMaxClient struct {
	httpClient       *http.Client
	baseURL         string
	apiKey          string
	model           string
	embeddingURL    string
	embeddingKey    string
	embeddingModel  string
	localOCRBaseURL string
	localOCRModel   string
}

func NewMiniMaxClient(cfg *config.LLMConfig) port.LLMClient {
	return &MiniMaxClient{
		httpClient:       &http.Client{Timeout: 120 * time.Second},
		baseURL:         "https://api.minimaxi.com/anthropic",
		apiKey:          cfg.APIKey,
		model:           cfg.Model,
		embeddingURL:    cfg.OllamaBaseURL,
		embeddingKey:    cfg.APIKey,
		embeddingModel:  cfg.EmbeddingModel,
		localOCRBaseURL: cfg.LocalOCRBaseURL,
		localOCRModel:   cfg.LocalOCRModel,
	}
}

type anthropicRequest struct {
	Model     string                   `json:"model"`
	MaxTokens int64                    `json:"max_tokens"`
	System    string                   `json:"system,omitempty"`
	Messages  []anthropicMessageContent `json:"messages"`
	Stream    bool                     `json:"stream,omitempty"`
}

type anthropicMessageContent struct {
	Role    string              `json:"role"`
	Content []anthropicTextBlock `json:"content"`
}

type anthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicResponse struct {
	Type     string                  `json:"type"`
	Content  []anthropicContentBlock `json:"content"`
	StopReason string                `json:"stop_reason"`
}

type anthropicContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking,omitempty"`
}

func (c *MiniMaxClient) chatRequest(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []anthropicMessageContent{
			{
				Role: "user",
				Content: []anthropicTextBlock{
					{Type: "text", Text: userPrompt},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal response failed: %w", err)
	}

	// 遍历 content 找到 text 类型
	var text string
	for _, c := range result.Content {
		if c.Type == "text" {
			text = c.Text
			break
		}
	}

	if text == "" {
		return "", fmt.Errorf("no text content in response")
	}

	return text, nil
}

func (c *MiniMaxClient) ExtractInfo(ctx context.Context, caseContent string) (string, error) {
	prompt := fmt.Sprintf(`你是一个专业的法律文书分析助手。请从以下案卷内容中提取关键信息，并以JSON格式返回。

案卷内容：
%s

请提取以下信息：
1. violation_facts: 违法事实摘要（简洁描述主要违法行为）
2. cited_laws: 案卷中引用的法律法规条文列表（完整的法规名称和条款号）
3. key_facts: 关键事实要素列表（如时间、地点、当事人、违法行为等）

必须严格按照以下JSON格式输出，不要有任何其他文字：
{
  "violation_facts": "违法事实摘要",
  "cited_laws": ["《法规名称》第X条", "《法规名称》第Y条"],
  "key_facts": ["关键事实1", "关键事实2"]
}`, caseContent)

	content, err := c.chatRequest(ctx, "你是一个专业的法律文书分析助手，必须严格按照JSON格式输出。", prompt)
	if err != nil {
		return "", err
	}
	return extractJSON(content), nil
}

func (c *MiniMaxClient) ValidateLaws(ctx context.Context, facts, citedLaws, retrievedLaws string) (string, error) {
	prompt := fmt.Sprintf(`你是一个专业的法律审核专家。请根据以下信息，判断案卷中引用的法律条文是否正确，并指出存在的问题。

违法事实：
%s

案卷引用的法条：
%s

检索到的相关法规：
%s

请分析：
1. 案卷引用的法条是否存在且有效（未废止）
2. 引用的法条是否适用于该违法事实
3. 是否存在引用错误、遗漏或不当之处

必须严格按照以下JSON格式输出，不要有任何其他文字：
{
  "is_correct": true/false,
  "issues": [
    {
      "title": "问题标题",
      "description": "详细描述",
      "severity": "critical/high/medium/low",
      "related_doc": "相关文档名称",
      "reference_law": "正确的法律引用"
    }
  ],
  "risk_score": 0-100
}

如果没有问题，issues数组为空，risk_score为0。`, facts, citedLaws, retrievedLaws)

	content, err := c.chatRequest(ctx, "你是一个专业的法律审核专家，必须严格按照JSON格式输出。", prompt)
	if err != nil {
		return "", err
	}
	return extractJSON(content), nil
}

func (c *MiniMaxClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// 使用 OpenAI 兼容接口生成 embedding
	return createEmbeddingWithOpenAI(ctx, text, c.embeddingURL, c.embeddingKey, c.embeddingModel)
}

func (c *MiniMaxClient) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return c.chatRequest(ctx, systemPrompt, userPrompt)
}

func (c *MiniMaxClient) ChatWithHistory(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	var systemContent string
	var msgContents []anthropicMessageContent

	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			systemContent = msg.Content
		case openai.ChatMessageRoleUser:
			msgContents = append(msgContents, anthropicMessageContent{
				Role: "user",
				Content: []anthropicTextBlock{{Type: "text", Text: msg.Content}},
			})
		case openai.ChatMessageRoleAssistant:
			msgContents = append(msgContents, anthropicMessageContent{
				Role: "assistant",
				Content: []anthropicTextBlock{{Type: "text", Text: msg.Content}},
			})
		}
	}

	if systemContent == "" {
		systemContent = "你是一个专业的法律助手。"
	}

	// 使用新的 API 格式
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemContent,
		Messages:  msgContents,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal response failed: %w", err)
	}

	// 遍历 content 找到 text 类型
	var text string
	for _, c := range result.Content {
		if c.Type == "text" {
			text = c.Text
			break
		}
	}

	if text == "" {
		return "", fmt.Errorf("no text content in response")
	}

	return text, nil
}

func (c *MiniMaxClient) RewriteQuery(ctx context.Context, ocrText, rule string) (*port.QueryRewriteResult, error) {
	prompt := fmt.Sprintf(`你是一个法律检索助手。用户提供了 OCR 评审规则，请帮助识别的文本和提取检索所需的信息。

OCR 文本：
%s

评审规则：
%s

请执行以下操作：
1. 修正 OCR 文本中的明显错误（如错别字、乱码）
2. 提取 3-5 个核心搜索关键词（用于全文检索，应为法律术语）
3. 生成一段简短的、标准的法言法语描述（用于向量检索，50字以内）
4. 如果规则中隐含了特定的法律法规范围（如《消防法》《治安管理处罚法》），请提取出来

必须严格按照以下JSON格式输出，不要有任何其他文字：
{
  "keywords": ["关键词1", "关键词2", "关键词3"],
  "normalized_query": "规范化的法律描述",
  "law_category": "法律类别（如无则为空字符串）",
  "corrected_ocr_text": "修正后的OCR文本"
}`, ocrText, rule)

	// 使用本地 OCR 模型进行查询重写，提高速度和降低成本
	reqBody := map[string]interface{}{
		"model": c.localOCRModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个法律检索助手，必须严格按照JSON格式输出。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.0,
		"max_tokens": 2000,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.localOCRBaseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to local OCR model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("local OCR API error: %s, body: %s", resp.Status, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from local OCR model")
	}

	content := extractJSON(apiResp.Choices[0].Message.Content)

	var result port.QueryRewriteResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse rewrite result: %w", err)
	}

	return &result, nil
}

func (c *MiniMaxClient) ChatStream(ctx context.Context, systemPrompt, userPrompt string, callback func(content string) error) error {
	return c.ChatWithHistoryStream(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	}, callback)
}

func (c *MiniMaxClient) ChatWithHistoryStream(ctx context.Context, messages []openai.ChatCompletionMessage, callback func(content string) error) error {
	var systemPrompt, userPrompt string

	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			systemPrompt = msg.Content
		case openai.ChatMessageRoleUser:
			userPrompt = msg.Content
		}
	}

	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []anthropicMessageContent{
			{
				Role: "user",
				Content: []anthropicTextBlock{
					{Type: "text", Text: userPrompt},
				},
			},
		},
		Stream: true,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	for {
		var event map[string]interface{}
		if err := decoder.Decode(&event); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("decode error: %w", err)
		}

		eventType, ok := event["type"].(string)
		if !ok {
			continue
		}

		if eventType == "content_block_delta" {
			delta, ok := event["delta"].(map[string]interface{})
			if !ok {
				continue
			}
			text, ok := delta["text"].(string)
			if !ok {
				continue
			}
			if err := callback(text); err != nil {
				return err
			}
		}
	}

	return nil
}

// createEmbeddingWithOpenAI 使用 OpenAI 兼容接口生成 embedding
func createEmbeddingWithOpenAI(ctx context.Context, text, baseURL, apiKey, model string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"input": text,
		"model": model,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}
