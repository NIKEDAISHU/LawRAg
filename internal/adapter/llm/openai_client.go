package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/config"
	"text/template"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client         *openai.Client
	model          string
	embeddingModel string
}

func NewOpenAIClient(cfg *config.LLMConfig) port.LLMClient {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL

	return &OpenAIClient{
		client:         openai.NewClientWithConfig(clientConfig),
		model:          cfg.Model,
		embeddingModel: cfg.EmbeddingModel,
	}
}

const extractInfoTemplate = `你是一个专业的法律文书分析助手。请从以下案卷内容中提取关键信息，并以JSON格式返回。

案卷内容：
{{.CaseContent}}

请提取以下信息：
1. violation_facts: 违法事实摘要（简洁描述主要违法行为）
2. cited_laws: 案卷中引用的法律法规条文列表（完整的法规名称和条款号）
3. key_facts: 关键事实要素列表（如时间、地点、当事人、违法行为等）

必须严格按照以下JSON格式输出，不要有任何其他文字：
{
  "violation_facts": "违法事实摘要",
  "cited_laws": ["《法规名称》第X条", "《法规名称》第Y条"],
  "key_facts": ["关键事实1", "关键事实2"]
}`

const validateLawsTemplate = `你是一个专业的法律审核专家。请根据以下信息，判断案卷中引用的法律条文是否正确，并指出存在的问题。

违法事实：
{{.Facts}}

案卷引用的法条：
{{.CitedLaws}}

检索到的相关法规：
{{.RetrievedLaws}}

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

如果没有问题，issues数组为空，risk_score为0。`

const rewriteQueryTemplate = `你是一个法律检索助手。用户提供了 OCR 识别的文本和评审规则，请帮助提取检索所需的信息。

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
}`

func (c *OpenAIClient) ExtractInfo(ctx context.Context, caseContent string) (string, error) {
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

func (c *OpenAIClient) ValidateLaws(ctx context.Context, facts string, citedLaws string, retrievedLaws string) (string, error) {
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

func (c *OpenAIClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// 添加超时控制，embedding 通常比较快，30 秒应该足够
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(c.embeddingModel),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create embedding: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return resp.Data[0].Embedding, nil
}

func (c *OpenAIClient) Chat(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
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

func (c *OpenAIClient) ChatWithHistory(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	// 添加超时控制，chat 可能需要更长时间，2 分钟应该足够
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.1,
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

func extractJSON(content string) string {
	start := -1
	end := -1

	for i, ch := range content {
		if ch == '{' && start == -1 {
			start = i
		}
		if ch == '}' {
			end = i + 1
		}
	}

	if start != -1 && end != -1 && end > start {
		jsonStr := content[start:end]
		var js json.RawMessage
		if err := json.Unmarshal([]byte(jsonStr), &js); err == nil {
			return jsonStr
		}
	}

	return content
}

func (c *OpenAIClient) RewriteQuery(ctx context.Context, ocrText string, rule string) (*port.QueryRewriteResult, error) {
	prompt := fmt.Sprintf(rewriteQueryTemplate, ocrText, rule)
	resp, err := c.Chat(ctx, "你是一个法律检索助手，必须严格按照JSON格式输出。", prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to rewrite query: %w", err)
	}

	var result port.QueryRewriteResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse rewrite result: %w", err)
	}

	return &result, nil
}

// ChatStream 流式聊天
func (c *OpenAIClient) ChatStream(ctx context.Context, systemPrompt string, userPrompt string, callback func(content string) error) error {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	}
	return c.ChatWithHistoryStream(ctx, messages, callback)
}

// ChatWithHistoryStream 支持多轮对话的流式聊天
func (c *OpenAIClient) ChatWithHistoryStream(ctx context.Context, messages []openai.ChatCompletionMessage, callback func(content string) error) error {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.1,
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
