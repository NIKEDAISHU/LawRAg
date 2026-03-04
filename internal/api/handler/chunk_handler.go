package handler

import (
	"net/http"
	"strconv"
	"time"

	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ChunkHandler Chunk 管理 Handler
type ChunkHandler struct {
	chunkManager port.ChunkManager
}

// NewChunkHandler 创建 Chunk Handler
func NewChunkHandler(chunkManager port.ChunkManager) *ChunkHandler {
	return &ChunkHandler{chunkManager: chunkManager}
}

// ListChunksRequest 列表请求
type ListChunksRequest struct {
	ProjectID int64  `form:"project_id"`
	DocName   string `form:"doc_name"`
	Page      int    `form:"page,default:1"`
	PageSize  int    `form:"page_size,default:20"`
}

// ChunkResponse Chunk 响应
type ChunkResponse struct {
	ID        int64                  `json:"id"`
	Content   string                 `json:"content"`
	QAContent string                 `json:"qa_content"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
}

// ListChunksResponse 列表响应
type ListChunksResponse struct {
	Total   int64           `json:"total"`
	Chunks  []ChunkResponse `json:"chunks"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}

// ListChunks 获取 chunks 列表
func (h *ChunkHandler) ListChunks(c *gin.Context) {
	var req ListChunksRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.Log.Error("Failed to bind request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	chunks, total, err := h.chunkManager.ListChunks(c.Request.Context(), req.ProjectID, req.DocName, req.Page, req.PageSize)
	if err != nil {
		logger.Log.Error("Failed to list chunks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := ListChunksResponse{
		Total:   total,
		Page:    req.Page,
		PerPage: req.PageSize,
		Chunks:  make([]ChunkResponse, len(chunks)),
	}

	for i, chunk := range chunks {
		response.Chunks[i] = ChunkResponse{
			ID:        chunk.ID,
			Content:   chunk.Content,
			QAContent: chunk.QAContent,
			Metadata:  chunk.Metadata,
			CreatedAt: chunk.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetChunk 获取单个 chunk
func (h *ChunkHandler) GetChunk(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk id"})
		return
	}

	chunk, err := h.chunkManager.GetChunkByID(c.Request.Context(), id)
	if err != nil {
		logger.Log.Error("Failed to get chunk", zap.Error(err), zap.Int64("id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "chunk not found"})
		return
	}

	response := ChunkResponse{
		ID:        chunk.ID,
		Content:   chunk.Content,
		QAContent: chunk.QAContent,
		Metadata:  chunk.Metadata,
		CreatedAt: chunk.CreatedAt,
	}

	c.JSON(http.StatusOK, response)
}

// UpdateChunkContentRequest 更新内容请求
type UpdateChunkContentRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateChunkContent 更新 chunk 内容
func (h *ChunkHandler) UpdateChunkContent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk id"})
		return
	}

	var req UpdateChunkContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Failed to bind request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 同步更新数据库
	if err := h.chunkManager.UpdateChunkContent(c.Request.Context(), id, req.Content); err != nil {
		logger.Log.Error("Failed to update chunk content", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 异步重新索引（如果需要）
	go func() {
		// 等待确保数据库更新完成
		time.Sleep(500 * time.Millisecond)
		// 这里可以添加异步向量化任务
	}()

	c.JSON(http.StatusOK, gin.H{"message": "updated successfully"})
}

// UpdateChunkQARequest 更新 QA 请求
type UpdateChunkQARequest struct {
	QAContent string `json:"qa_content"`
}

// UpdateChunkQA 更新 chunk QA 内容
func (h *ChunkHandler) UpdateChunkQA(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk id"})
		return
	}

	var req UpdateChunkQARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Failed to bind request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.chunkManager.UpdateChunkQA(c.Request.Context(), id, req.QAContent); err != nil {
		logger.Log.Error("Failed to update chunk QA", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated successfully"})
}

// RegenerateQA 重新生成 QA
func (h *ChunkHandler) RegenerateQA(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk id"})
		return
	}

	qa, err := h.chunkManager.RegenerateQA(c.Request.Context(), id)
	if err != nil {
		logger.Log.Error("Failed to regenerate QA", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"qa_content": qa})
}

// DeleteChunk 删除 chunk
func (h *ChunkHandler) DeleteChunk(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk id"})
		return
	}

	if err := h.chunkManager.DeleteChunk(c.Request.Context(), id); err != nil {
		logger.Log.Error("Failed to delete chunk", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}
