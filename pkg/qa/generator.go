package qa

import (
	"context"
	"fmt"
	"law-enforcement-brain/internal/core/port"
	"strings"
	"sync"
)

// Generator QA 生成器
type Generator struct {
	llmClient port.LLMClient
}

// NewGenerator 创建 QA 生成器
func NewGenerator(llmClient port.LLMClient) *Generator {
	return &Generator{llmClient: llmClient}
}

// GenerateQA 生成 QA 对
type GenerateQA struct {
	ChunkID   int64
	Content   string
	DocName   string
	Knowledge string // 知识库名称
}

// GenerateForChunks 批量生成 QA 对
func (g *Generator) GenerateForChunks(ctx context.Context, chunks []GenerateQA) (map[int64]string, error) {
	results := make(map[int64]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, chunk := range chunks {
		wg.Add(1)
		go func(c GenerateQA) {
			defer wg.Done()
			qa, err := g.generateQAForChunk(ctx, c)
			if err != nil {
				return
			}
			mu.Lock()
			results[c.ChunkID] = qa
			mu.Unlock()
		}(chunk)
	}
	wg.Wait()

	return results, nil
}

func (g *Generator) generateQAForChunk(ctx context.Context, chunk GenerateQA) (string, error) {
	knowledgeName := chunk.Knowledge
	if knowledgeName == "" {
		knowledgeName = chunk.DocName
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的问题生成助手，任务是从给定的法律文本中提取或生成可能的问题。你不需要回答这些问题，只需生成问题本身。

知识库名字是：《%s》

输出格式：
- 每个问题占一行
- 问题必须以问号结尾
- 避免重复或语义相似的问题

生成规则：
- 生成的问题必须严格基于文本内容，不能脱离文本虚构。
- 优先生成事实性问题（如谁、何时、何地、如何、违反了什么规定等）。
- 对于复杂文本，可生成多层次问题（基础事实 + 推理问题）。
- 禁止生成主观或开放式问题（如"你认为...？"）。
- 数量控制在3-5个

必须严格按照以上规则生成问题，不要有任何其他说明文字。`, knowledgeName)

	qa, err := g.llmClient.Chat(ctx, systemPrompt, chunk.Content)
	if err != nil {
		return "", err
	}

	return g.normalizeQA(qa), nil
}

func (g *Generator) normalizeQA(qa string) string {
	lines := strings.Split(qa, "\n")
	var result []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳过空行
		if line == "" {
			continue
		}
		// 跳过不是问题的行
		if !strings.HasSuffix(line, "?") && !strings.HasSuffix(line, "？") {
			continue
		}
		// 跳过明显是说明文字的行
		if strings.Contains(line, "问题") && len(line) < 10 {
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// ExtractQAQuestions 从 QA 内容中提取问题列表
func ExtractQAQuestions(qaContent string) []string {
	if qaContent == "" {
		return nil
	}

	questions := strings.Split(qaContent, "\n")
	var result []string

	for _, q := range questions {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		result = append(result, q)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
