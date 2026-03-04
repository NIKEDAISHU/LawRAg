package port

import (
	"context"
	"law-enforcement-brain/internal/core/domain"
)

// ReviewState 案件评审状态
// 在 Pipeline 中在各 Step 之间传递
type ReviewState struct {
	// 输入
	CaseContent string
	ProjectIDs  []int64
	DocList     []domain.Document // 文档列表

	// Step 1: 提取关键信息
	ExtractedInfo *domain.ExtractedInfo

	// Step 2: 检索相关法规
	RetrievedLaws    string
	RetrievedChunks []domain.LawChunk

	// Step 3: 法律验证
	ValidationResult *domain.ValidationResult

	// Step 4: 结果
	FinalResult *domain.ReviewResult

	// 元数据
	Metadata map[string]interface{}
}

// ReviewStep 案件评审流水线步骤接口
type ReviewStep interface {
	// Name 步骤名称
	Name() string
	// Execute 执行步骤
	Execute(ctx context.Context, state *ReviewState) error
	// Skip 条件跳过
	Skip(ctx context.Context, state *ReviewState) bool
}

// ReviewPipeline 案件评审流水线
type ReviewPipeline struct {
	steps []ReviewStep
}

// NewReviewPipeline 创建评审流水线
func NewReviewPipeline(steps []ReviewStep) *ReviewPipeline {
	return &ReviewPipeline{
		steps: steps,
	}
}

// Execute 执行整个流水线
func (p *ReviewPipeline) Execute(ctx context.Context, state *ReviewState) error {
	for _, step := range p.steps {
		if step.Skip(ctx, state) {
			continue
		}

		if err := step.Execute(ctx, state); err != nil {
			return err
		}
	}
	return nil
}

// AddStep 添加步骤
func (p *ReviewPipeline) AddStep(step ReviewStep) {
	p.steps = append(p.steps, step)
}

// RetrievalStrategy 检索策略接口
type RetrievalStrategy interface {
	Retrieve(ctx context.Context, state *ReviewState) ([]domain.LawChunk, error)
	RetrieveDirect(ctx context.Context, query string, projectIDs []int64) ([]domain.LawChunk, error)
	Name() string
}

// RetrievalStrategyType 检索策略类型
type RetrievalStrategyType string

const (
	RetrievalStrategyHybrid    RetrievalStrategyType = "hybrid"
	RetrievalStrategyVector    RetrievalStrategyType = "vector"
	RetrievalStrategyKeyword   RetrievalStrategyType = "keyword"
	RetrievalStrategyAdaptive  RetrievalStrategyType = "adaptive"
)
