package port

import (
	"context"
	"law-enforcement-brain/internal/core/domain"
)

// ChunkManager Chunk 管理接口
type ChunkManager interface {
	// ListChunks 获取 chunks 列表
	ListChunks(ctx context.Context, projectID int64, docName string, page, pageSize int) ([]domain.LawChunk, int64, error)

	// GetChunkByID 根据 ID 获取 chunk
	GetChunkByID(ctx context.Context, id int64) (*domain.LawChunk, error)

	// UpdateChunkContent 更新 chunk 内容
	UpdateChunkContent(ctx context.Context, id int64, content string) error

	// UpdateChunkQA 更新 chunk QA 内容
	UpdateChunkQA(ctx context.Context, id int64, qaContent string) error

	// DeleteChunk 删除 chunk
	DeleteChunk(ctx context.Context, id int64) error

	// BatchDeleteChunks 批量删除 chunks
	BatchDeleteChunks(ctx context.Context, ids []int64) error

	// RegenerateQA 重新生成 QA
	RegenerateQA(ctx context.Context, id int64) (string, error)
}
