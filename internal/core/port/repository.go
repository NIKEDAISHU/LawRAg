package port

import (
	"context"
	"law-enforcement-brain/internal/core/domain"
)

type ProjectRepository interface {
	CreateProject(ctx context.Context, req domain.CreateProjectRequest) (*domain.Project, error)
	GetProject(ctx context.Context, id int64) (*domain.Project, error)
	GetProjectByCode(ctx context.Context, code string) (*domain.Project, error)
	ListProjects(ctx context.Context, activeOnly bool) ([]domain.Project, error)
	UpdateProject(ctx context.Context, id int64, req domain.UpdateProjectRequest) (*domain.Project, error)
	DeleteProject(ctx context.Context, id int64) error
	GetProjectStatistics(ctx context.Context, id int64) (*domain.ProjectStatistics, error)
	ListProjectStatistics(ctx context.Context, activeOnly bool) ([]domain.ProjectStatistics, error)
}

type LawRepository interface {
	SearchSimilar(ctx context.Context, projectIDs []int64, query string, topK int) ([]domain.LawChunk, error)
	SearchByKeywords(ctx context.Context, projectIDs []int64, keywords []string) ([]domain.LawChunk, error)
	Store(ctx context.Context, projectID int64, content string, vector []float32, metadata map[string]string) error
	StoreWithQA(ctx context.Context, projectID int64, content string, vector []float32, qaContent string, qaVector []float32, metadata map[string]string) error
	HybridSearch(ctx context.Context, projectIDs []int64, query string, keywords []string, topK int) ([]domain.LawChunk, error)
	InsertChunk(ctx context.Context, projectID int64, chunk domain.LawChunk) error
	BatchInsertChunks(ctx context.Context, projectID int64, chunks []domain.LawChunk) error
	GetChunkByID(ctx context.Context, id int64) (*domain.LawChunk, error)
	UpdateChunk(ctx context.Context, projectID int64, chunk domain.LawChunk) error
	DeleteChunk(ctx context.Context, id int64) error
	GetLawDocuments(ctx context.Context, projectIDs []int64) ([]domain.LawDocument, error)
	GetLawArticles(ctx context.Context, projectID int64, docName string) ([]domain.LawChunk, error)
}
