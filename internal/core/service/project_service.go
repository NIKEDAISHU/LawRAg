package service

import (
	"context"
	"fmt"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
)

type ProjectService struct {
	projectRepo port.ProjectRepository
}

func NewProjectService(projectRepo port.ProjectRepository) *ProjectService {
	return &ProjectService{
		projectRepo: projectRepo,
	}
}

func (s *ProjectService) CreateProject(ctx context.Context, req domain.CreateProjectRequest) (*domain.Project, error) {
	if req.ProjectCode == "" {
		return nil, fmt.Errorf("project_code is required")
	}
	if req.ProjectName == "" {
		return nil, fmt.Errorf("project_name is required")
	}

	existing, _ := s.projectRepo.GetProjectByCode(ctx, req.ProjectCode)
	if existing != nil {
		return nil, fmt.Errorf("project with code '%s' already exists", req.ProjectCode)
	}

	if req.EmbeddingModel == "" {
		req.EmbeddingModel = "bge-m3"
	}
	if req.VectorDim == 0 {
		req.VectorDim = 1024
	}
	if req.ProjectType == "" {
		req.ProjectType = "general"
	}

	return s.projectRepo.CreateProject(ctx, req)
}

func (s *ProjectService) GetProject(ctx context.Context, id int64) (*domain.Project, error) {
	return s.projectRepo.GetProject(ctx, id)
}

func (s *ProjectService) GetProjectByCode(ctx context.Context, code string) (*domain.Project, error) {
	return s.projectRepo.GetProjectByCode(ctx, code)
}

func (s *ProjectService) ListProjects(ctx context.Context, activeOnly bool) ([]domain.Project, error) {
	return s.projectRepo.ListProjects(ctx, activeOnly)
}

func (s *ProjectService) UpdateProject(ctx context.Context, id int64, req domain.UpdateProjectRequest) (*domain.Project, error) {
	existing, err := s.projectRepo.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("project not found")
	}

	return s.projectRepo.UpdateProject(ctx, id, req)
}

func (s *ProjectService) DeleteProject(ctx context.Context, id int64) error {
	return s.projectRepo.DeleteProject(ctx, id)
}

func (s *ProjectService) GetProjectStatistics(ctx context.Context, id int64) (*domain.ProjectStatistics, error) {
	return s.projectRepo.GetProjectStatistics(ctx, id)
}

func (s *ProjectService) ListProjectStatistics(ctx context.Context, activeOnly bool) ([]domain.ProjectStatistics, error) {
	return s.projectRepo.ListProjectStatistics(ctx, activeOnly)
}

func (s *ProjectService) ValidateProjectIDs(ctx context.Context, projectIDs []int64) error {
	if len(projectIDs) == 0 {
		return nil
	}

	for _, id := range projectIDs {
		project, err := s.projectRepo.GetProject(ctx, id)
		if err != nil {
			return fmt.Errorf("project %d not found: %w", id, err)
		}
		if !project.IsActive {
			return fmt.Errorf("project %d is not active", id)
		}
	}

	return nil
}

func (s *ProjectService) ValidateEmbeddingModelConsistency(ctx context.Context, projectIDs []int64) error {
	if len(projectIDs) <= 1 {
		return nil
	}

	var embeddingModel string
	var vectorDim int

	for i, id := range projectIDs {
		project, err := s.projectRepo.GetProject(ctx, id)
		if err != nil {
			return fmt.Errorf("project %d not found: %w", id, err)
		}

		if i == 0 {
			embeddingModel = project.EmbeddingModel
			vectorDim = project.VectorDim
		} else {
			if project.EmbeddingModel != embeddingModel {
				return fmt.Errorf("inconsistent embedding models: project %d uses '%s', but project %d uses '%s'",
					projectIDs[0], embeddingModel, id, project.EmbeddingModel)
			}
			if project.VectorDim != vectorDim {
				return fmt.Errorf("inconsistent vector dimensions: project %d uses %d, but project %d uses %d",
					projectIDs[0], vectorDim, id, project.VectorDim)
			}
		}
	}

	return nil
}
