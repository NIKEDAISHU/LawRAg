package handler

import (
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type KnowledgeHandler struct {
	lawRepo port.LawRepository
}

func NewKnowledgeHandler(lawRepo port.LawRepository) *KnowledgeHandler {
	return &KnowledgeHandler{
		lawRepo: lawRepo,
	}
}

func (h *KnowledgeHandler) GetLawDocuments(c *gin.Context) {
	ctx := c.Request.Context()

	// 从查询参数获取项目ID，默认为1
	projectIDStr := c.DefaultQuery("project_id", "1")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		projectID = domain.DefaultProjectID
	}
	projectIDs := []int64{projectID}

	docs, err := h.lawRepo.GetLawDocuments(ctx, projectIDs)
	if err != nil {
		logger.Log.Error("Failed to get law documents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取法律文档列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data": gin.H{
			"documents": docs,
			"total":     len(docs),
		},
	})
}

func (h *KnowledgeHandler) GetLawArticles(c *gin.Context) {
	docName := c.Param("doc_name")
	if docName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "文档名称不能为空",
		})
		return
	}

	ctx := c.Request.Context()

	// 从查询参数获取项目ID，默认为1
	projectIDStr := c.DefaultQuery("project_id", "1")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		projectID = domain.DefaultProjectID
	}

	articles, err := h.lawRepo.GetLawArticles(ctx, projectID, docName)
	if err != nil {
		logger.Log.Error("Failed to get law articles",
			zap.String("doc_name", docName),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取法律条文失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data": gin.H{
			"doc_name": docName,
			"articles": articles,
			"total":    len(articles),
		},
	})
}
