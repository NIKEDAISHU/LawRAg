package repository

import (
	"context"
	"fmt"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"time"

	"gorm.io/gorm"
)

type ProjectModel struct {
	ID             int64                  `gorm:"primaryKey;column:id"`
	ProjectCode    string                 `gorm:"column:project_code;type:varchar(50);uniqueIndex;not null"`
	ProjectName    string                 `gorm:"column:project_name;type:varchar(100);not null"`
	ProjectType    string                 `gorm:"column:project_type;type:varchar(50);not null;default:'general'"`
	Description    string                 `gorm:"column:description;type:text"`
	EmbeddingModel string                 `gorm:"column:embedding_model;type:varchar(100);not null;default:'bge-m3'"`
	VectorDim      int                    `gorm:"column:vector_dim;not null;default:1024"`
	Config         map[string]interface{} `gorm:"column:config;type:jsonb;serializer:json"`
	IsActive       bool                   `gorm:"column:is_active;default:true"`
	CreatedAt      time.Time              `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time              `gorm:"column:updated_at;autoUpdateTime"`
}

func (ProjectModel) TableName() string {
	return "projects"
}

type ProjectStatisticsView struct {
	ID             int64                  `gorm:"column:id"`
	ProjectCode    string                 `gorm:"column:project_code"`
	ProjectName    string                 `gorm:"column:project_name"`
	ProjectType    string                 `gorm:"column:project_type"`
	IsActive       bool                   `gorm:"column:is_active"`
	DocCount       int                    `gorm:"column:doc_count"`
	ChunkCount     int                    `gorm:"column:chunk_count"`
	EmbeddingModel string                 `gorm:"column:embedding_model"`
	VectorDim      int                    `gorm:"column:vector_dim"`
	Config         map[string]interface{} `gorm:"column:config;type:jsonb;serializer:json"`
	CreatedAt      time.Time              `gorm:"column:created_at"`
	UpdatedAt      time.Time              `gorm:"column:updated_at"`
}

type ProjectRepositoryImpl struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) port.ProjectRepository {
	return &ProjectRepositoryImpl{db: db}
}

func (r *ProjectRepositoryImpl) CreateProject(ctx context.Context, req domain.CreateProjectRequest) (*domain.Project, error) {
	model := ProjectModel{
		ProjectCode:    req.ProjectCode,
		ProjectName:    req.ProjectName,
		ProjectType:    req.ProjectType,
		Description:    req.Description,
		EmbeddingModel: req.EmbeddingModel,
		VectorDim:      req.VectorDim,
		Config:         req.Config,
		IsActive:       true,
	}

	if model.ProjectType == "" {
		model.ProjectType = "general"
	}
	if model.EmbeddingModel == "" {
		model.EmbeddingModel = "bge-m3"
	}
	if model.VectorDim == 0 {
		model.VectorDim = 1024
	}
	if model.Config == nil {
		model.Config = make(map[string]interface{})
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return r.modelToDomain(&model), nil
}

func (r *ProjectRepositoryImpl) GetProject(ctx context.Context, id int64) (*domain.Project, error) {
	var model ProjectModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("project not found")
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return r.modelToDomain(&model), nil
}

func (r *ProjectRepositoryImpl) GetProjectByCode(ctx context.Context, code string) (*domain.Project, error) {
	var model ProjectModel
	if err := r.db.WithContext(ctx).Where("project_code = ?", code).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("project not found")
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return r.modelToDomain(&model), nil
}

func (r *ProjectRepositoryImpl) ListProjects(ctx context.Context, activeOnly bool) ([]domain.Project, error) {
	var models []ProjectModel
	query := r.db.WithContext(ctx)
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	projects := make([]domain.Project, len(models))
	for i, model := range models {
		projects[i] = *r.modelToDomain(&model)
	}
	return projects, nil
}

func (r *ProjectRepositoryImpl) UpdateProject(ctx context.Context, id int64, req domain.UpdateProjectRequest) (*domain.Project, error) {
	updates := make(map[string]interface{})

	if req.ProjectName != "" {
		updates["project_name"] = req.ProjectName
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Config != nil {
		updates["config"] = req.Config
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		return r.GetProject(ctx, id)
	}

	if err := r.db.WithContext(ctx).Model(&ProjectModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return r.GetProject(ctx, id)
}

func (r *ProjectRepositoryImpl) DeleteProject(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&ProjectModel{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

func (r *ProjectRepositoryImpl) GetProjectStatistics(ctx context.Context, id int64) (*domain.ProjectStatistics, error) {
	var stat ProjectStatisticsView

	err := r.db.WithContext(ctx).
		Table("projects p").
		Select(`
			p.id, p.project_code, p.project_name, p.project_type, p.is_active,
			p.embedding_model, p.vector_dim, p.config, p.created_at, p.updated_at,
			COUNT(DISTINCT kc.doc_name) as doc_count,
			COUNT(kc.id) as chunk_count
		`).
		Joins("LEFT JOIN knowledge_chunks kc ON p.id = kc.project_id").
		Where("p.id = ?", id).
		Group("p.id, p.project_code, p.project_name, p.project_type, p.is_active, p.embedding_model, p.vector_dim, p.config, p.created_at, p.updated_at").
		Scan(&stat).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get project statistics: %w", err)
	}

	return &domain.ProjectStatistics{
		Project: domain.Project{
			ID:             stat.ID,
			ProjectCode:    stat.ProjectCode,
			ProjectName:    stat.ProjectName,
			ProjectType:    stat.ProjectType,
			EmbeddingModel: stat.EmbeddingModel,
			VectorDim:      stat.VectorDim,
			Config:         stat.Config,
			IsActive:       stat.IsActive,
			CreatedAt:      stat.CreatedAt,
			UpdatedAt:      stat.UpdatedAt,
		},
		DocCount:   stat.DocCount,
		ChunkCount: stat.ChunkCount,
	}, nil
}

func (r *ProjectRepositoryImpl) ListProjectStatistics(ctx context.Context, activeOnly bool) ([]domain.ProjectStatistics, error) {
	var stats []ProjectStatisticsView

	query := r.db.WithContext(ctx).
		Table("projects p").
		Select(`
			p.id, p.project_code, p.project_name, p.project_type, p.is_active,
			p.embedding_model, p.vector_dim, p.config, p.created_at, p.updated_at,
			COUNT(DISTINCT kc.doc_name) as doc_count,
			COUNT(kc.id) as chunk_count
		`).
		Joins("LEFT JOIN knowledge_chunks kc ON p.id = kc.project_id").
		Group("p.id, p.project_code, p.project_name, p.project_type, p.is_active, p.embedding_model, p.vector_dim, p.config, p.created_at, p.updated_at")

	if activeOnly {
		query = query.Where("p.is_active = ?", true)
	}

	if err := query.Order("p.created_at DESC").Scan(&stats).Error; err != nil {
		return nil, fmt.Errorf("failed to list project statistics: %w", err)
	}

	result := make([]domain.ProjectStatistics, len(stats))
	for i, stat := range stats {
		result[i] = domain.ProjectStatistics{
			Project: domain.Project{
				ID:             stat.ID,
				ProjectCode:    stat.ProjectCode,
				ProjectName:    stat.ProjectName,
				ProjectType:    stat.ProjectType,
				EmbeddingModel: stat.EmbeddingModel,
				VectorDim:      stat.VectorDim,
				Config:         stat.Config,
				IsActive:       stat.IsActive,
				CreatedAt:      stat.CreatedAt,
				UpdatedAt:      stat.UpdatedAt,
			},
			DocCount:   stat.DocCount,
			ChunkCount: stat.ChunkCount,
		}
	}
	return result, nil
}

func (r *ProjectRepositoryImpl) modelToDomain(model *ProjectModel) *domain.Project {
	return &domain.Project{
		ID:             model.ID,
		ProjectCode:    model.ProjectCode,
		ProjectName:    model.ProjectName,
		ProjectType:    model.ProjectType,
		Description:    model.Description,
		EmbeddingModel: model.EmbeddingModel,
		VectorDim:      model.VectorDim,
		Config:         model.Config,
		IsActive:       model.IsActive,
		CreatedAt:      model.CreatedAt,
		UpdatedAt:      model.UpdatedAt,
	}
}
