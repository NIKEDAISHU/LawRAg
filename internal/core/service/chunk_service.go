package service

import (
	"context"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/qa"
)

// ChunkService Chunk 管理服务
type ChunkService struct {
	vectorStore port.LawRepository
	llmClient   port.LLMClient
}

// NewChunkService 创建 Chunk 服务
func NewChunkService(vectorStore port.LawRepository, llmClient port.LLMClient) port.ChunkManager {
	return &ChunkService{
		vectorStore: vectorStore,
		llmClient:   llmClient,
	}
}

// ListChunks 获取 chunks 列表
func (s *ChunkService) ListChunks(ctx context.Context, projectID int64, docName string, page, pageSize int) ([]domain.LawChunk, int64, error) {
	offset := 0
	if page > 1 {
		offset = (page - 1) * pageSize
	}

	// 获取文档的所有 chunks
	chunks, err := s.vectorStore.GetLawArticles(ctx, projectID, docName)
	if err != nil {
		return nil, 0, err
	}

	total := int64(len(chunks))

	// 分页
	start := offset
	if start >= len(chunks) {
		return []domain.LawChunk{}, total, nil
	}
	end := start + pageSize
	if end > len(chunks) {
		end = len(chunks)
	}

	return chunks[start:end], total, nil
}

// GetChunkByID 根据 ID 获取 chunk - 直接调用 repository
func (s *ChunkService) GetChunkByID(ctx context.Context, id int64) (*domain.LawChunk, error) {
	return s.vectorStore.GetChunkByID(ctx, id)
}

// UpdateChunkContent 更新 chunk 内容
func (s *ChunkService) UpdateChunkContent(ctx context.Context, id int64, content string) error {
	chunk, err := s.GetChunkByID(ctx, id)
	if err != nil {
		return err
	}
	if chunk == nil {
		return nil
	}

	// 更新内容
	chunk.Content = content

	// 重新生成向量
	vector, err := s.llmClient.CreateEmbedding(ctx, content)
	if err != nil {
		return err
	}
	chunk.Vector = vector

	// 调用 repository 的 UpdateChunk 方法
	projectID := int64(0)
	if pid, ok := chunk.Metadata["project_id"].(int64); ok {
		projectID = pid
	}
	return s.vectorStore.UpdateChunk(ctx, projectID, *chunk)
}

// UpdateChunkQA 更新 chunk QA 内容
func (s *ChunkService) UpdateChunkQA(ctx context.Context, id int64, qaContent string) error {
	chunk, err := s.GetChunkByID(ctx, id)
	if err != nil {
		return err
	}
	if chunk == nil {
		return nil
	}

	chunk.QAContent = qaContent

	// 生成 QA 向量
	qaVector, err := s.llmClient.CreateEmbedding(ctx, qaContent)
	if err != nil {
		return err
	}
	chunk.QAVector = qaVector

	// 调用 repository 的 UpdateChunk 方法
	projectID := int64(0)
	if pid, ok := chunk.Metadata["project_id"].(int64); ok {
		projectID = pid
	}
	return s.vectorStore.UpdateChunk(ctx, projectID, *chunk)
}

// DeleteChunk 删除 chunk - 直接调用 repository
func (s *ChunkService) DeleteChunk(ctx context.Context, id int64) error {
	return s.vectorStore.DeleteChunk(ctx, id)
}

// BatchDeleteChunks 批量删除 chunks
func (s *ChunkService) BatchDeleteChunks(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if err := s.DeleteChunk(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// RegenerateQA 重新生成 QA
func (s *ChunkService) RegenerateQA(ctx context.Context, id int64) (string, error) {
	chunk, err := s.GetChunkByID(ctx, id)
	if err != nil {
		return "", err
	}
	if chunk == nil {
		return "", nil
	}

	// 使用 QA 生成器
	qaGenerator := qa.NewGenerator(s.llmClient)
	qaReq := qa.GenerateQA{
		ChunkID:   id,
		Content:   chunk.Content,
		DocName:   "",
		Knowledge: "",
	}
	qaResult, err := qaGenerator.GenerateForChunks(ctx, []qa.GenerateQA{qaReq})
	if err != nil {
		return "", err
	}
	if len(qaResult) == 0 {
		return "", nil
	}

	// 更新 QA 内容
	if err := s.UpdateChunkQA(ctx, id, qaResult[id]); err != nil {
		return "", err
	}

	return qaResult[id], nil
}
