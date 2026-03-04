package handler

import (
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/service"
	"law-enforcement-brain/pkg/logger"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ProjectHandler struct {
	projectService *service.ProjectService
}

func NewProjectHandler(projectService *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{
		projectService: projectService,
	}
}

func (h *ProjectHandler) CreateProject(c *gin.Context) {
	var req domain.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	project, err := h.projectService.CreateProject(ctx, req)
	if err != nil {
		logger.Log.Error("Failed to create project", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "创建项目失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "项目创建成功",
		"data":    project,
	})
}

func (h *ProjectHandler) GetProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	ctx := c.Request.Context()
	project, err := h.projectService.GetProject(ctx, id)
	if err != nil {
		logger.Log.Error("Failed to get project", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "项目不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    project,
	})
}

func (h *ProjectHandler) ListProjects(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"

	ctx := c.Request.Context()
	projects, err := h.projectService.ListProjects(ctx, activeOnly)
	if err != nil {
		logger.Log.Error("Failed to list projects", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取项目列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"projects": projects,
			"total":    len(projects),
		},
	})
}

func (h *ProjectHandler) UpdateProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	var req domain.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Error("Invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	project, err := h.projectService.UpdateProject(ctx, id, req)
	if err != nil {
		logger.Log.Error("Failed to update project", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新项目失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "项目更新成功",
		"data":    project,
	})
}

func (h *ProjectHandler) DeleteProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	ctx := c.Request.Context()
	if err := h.projectService.DeleteProject(ctx, id); err != nil {
		logger.Log.Error("Failed to delete project", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "删除项目失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "项目删除成功",
	})
}

func (h *ProjectHandler) GetProjectStatistics(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	ctx := c.Request.Context()
	stats, err := h.projectService.GetProjectStatistics(ctx, id)
	if err != nil {
		logger.Log.Error("Failed to get project statistics", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "获取项目统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

func (h *ProjectHandler) ListProjectStatistics(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"

	ctx := c.Request.Context()
	stats, err := h.projectService.ListProjectStatistics(ctx, activeOnly)
	if err != nil {
		logger.Log.Error("Failed to list project statistics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取项目统计列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"projects": stats,
			"total":    len(stats),
		},
	})
}
