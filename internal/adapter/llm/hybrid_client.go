package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/config"
	"net/http"
	"text/template"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type HybridClient struct {
	chatClient      *openai.Client
	chatModel       string
	ollamaBaseURL   string
	embeddingModel  string
	localOCRBaseURL string
	localOCRModel   string
	httpClient      *http.Client
}

func NewHybridClient(cfg *config.LLMConfig) port.LLMClient {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL

	return &HybridClient{
		chatClient:      openai.NewClientWithConfig(clientConfig),
		chatModel:       cfg.Model,
		ollamaBaseURL:   cfg.OllamaBaseURL,
		embeddingModel:  cfg.EmbeddingModel,
		localOCRBaseURL: cfg.LocalOCRBaseURL,
		localOCRModel:   cfg.LocalOCRModel,
		httpClient:      &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *HybridClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model":  c.embeddingModel,
		"prompt": text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.ollamaBaseURL+"/api/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error: %s, body: %s", resp.Status, string(body))
	}

	var embResp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	return embResp.Embedding, nil
}

func (c *HybridClient) Chat(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userPrompt,
		},
	}
	return c.ChatWithHistory(ctx, messages)
}

func (c *HybridClient) ChatWithHistory(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	// 设置 60 秒超时
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := c.chatClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.chatModel,
		Messages:    messages,
		Temperature: 0.1,
		MaxTokens:   2000,
	})

	if err != nil {
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	content := resp.Choices[0].Message.Content
	content = extractJSON(content)

	return content, nil
}

func (c *HybridClient) ExtractInfo(ctx context.Context, caseContent string) (string, error) {
	tmpl, err := template.New("extract").Parse(extractInfoTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"CaseContent": caseContent}); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return c.Chat(ctx, "你是一个专业的法律文书分析助手，必须严格按照JSON格式输出。", buf.String())
}

func (c *HybridClient) ValidateLaws(ctx context.Context, facts string, citedLaws string, retrievedLaws string) (string, error) {
	tmpl, err := template.New("validate").Parse(validateLawsTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	data := map[string]string{
		"Facts":         facts,
		"CitedLaws":     citedLaws,
		"RetrievedLaws": retrievedLaws,
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return c.Chat(ctx, "你是一个专业的法律审核专家，必须严格按照JSON格式输出。", buf.String())
}

func (c *HybridClient) RewriteQuery(ctx context.Context, ocrText string, rule string) (*port.QueryRewriteResult, error) {
	// 使用本地 OCR 模型进行查询重写，提高速度和降低成本
	prompt := fmt.Sprintf(rewriteQueryTemplate, ocrText, rule)

	reqBody := map[string]interface{}{
		"model": c.localOCRModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个法律检索助手，必须严格按照JSON格式输出。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.0,
		"max_tokens":  2000,
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

// ChatStream 流式聊天
func (c *HybridClient) ChatStream(ctx context.Context, systemPrompt string, userPrompt string, callback func(content string) error) error {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	}
	return c.ChatWithHistoryStream(ctx, messages, callback)
}

// ChatWithHistoryStream 支持多轮对话的流式聊天
func (c *HybridClient) ChatWithHistoryStream(ctx context.Context, messages []openai.ChatCompletionMessage, callback func(content string) error) error {
	stream, err := c.chatClient.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       c.chatModel,
		Messages:    messages,
		Temperature: 0.1,
		MaxTokens:   2000,
	})
	if err != nil {
		return fmt.Errorf("failed to create chat completion stream: %w", err)
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" || err == io.EOF {
				break
			}
			return fmt.Errorf("stream recv error: %w", err)
		}

		if len(resp.Choices) > 0 {
			content := resp.Choices[0].Delta.Content
			if len(content) > 0 {
				if err := callback(content); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
