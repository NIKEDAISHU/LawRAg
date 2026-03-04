package service

import (
	"context"
	"fmt"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/config"
	"law-enforcement-brain/pkg/logger"
	"law-enforcement-brain/pkg/utils"
	"strings"

	"go.uber.org/zap"
)

type ReviewService struct {
	pipeline     *port.ReviewPipeline
	lawRepo      port.LawRepository
	llmClient    port.LLMClient
	cfg          *config.ReviewConfig
	fallbackMode bool
	vectorCache  *utils.VectorCache
	defaultProjectIDs []int64
}

func NewReviewService(lawRepo port.LawRepository, llmClient port.LLMClient, cfg *config.ReviewConfig) *ReviewService {
	// 创建引用验证器
	citationVerifier := NewLawCitationVerifier(lawRepo)

	// 创建检索策略（使用自适应策略）
	projectIDs := []int64{1}
	hybridStrategy := NewHybridRetrievalStrategy(lawRepo, llmClient, projectIDs, 10)
	vectorStrategy := NewVectorRetrievalStrategy(lawRepo, llmClient, projectIDs, 10)
	keywordStrategy := NewKeywordRetrievalStrategy(lawRepo, projectIDs, 10)
	adaptiveStrategy := NewAdaptiveRetrievalStrategy(hybridStrategy, vectorStrategy, keywordStrategy)

	// 创建 Pipeline
	steps := []port.ReviewStep{
		NewExtractInfoStep(llmClient),
		NewRetrieveLawsStep(adaptiveStrategy),
		NewValidateLawsStep(llmClient),
		NewVerifyCitationsStep(citationVerifier),
		NewAssembleResultStep(),
	}
	pipeline := port.NewReviewPipeline(steps)

	return &ReviewService{
		pipeline:     pipeline,
		lawRepo:      lawRepo,
		llmClient:    llmClient,
		cfg:          cfg,
		fallbackMode: cfg.FallbackMode,
		vectorCache:  utils.NewVectorCache(),
		defaultProjectIDs: []int64{1},
	}
}

func (s *ReviewService) ReviewCase(ctx context.Context, req domain.ReviewRequest) (*domain.ReviewResult, error) {
	logger.Log.Info("Starting case review",
		zap.String("request_id", req.RequestID),
		zap.String("case_type", req.CaseInfo.CaseType),
		zap.Int("doc_count", len(req.CaseInfo.DocList)))

	// 组装案件内容
	caseContent := s.assembleCaseContent(req.CaseInfo.DocList)

	// 创建评审状态
	state := &port.ReviewState{
		CaseContent: caseContent,
		DocList:     req.CaseInfo.DocList,
		ProjectIDs:  s.defaultProjectIDs,
		Metadata:    make(map[string]interface{}),
	}

	// 执行 Pipeline
	if err := s.pipeline.Execute(ctx, state); err != nil {
		logger.Log.Warn("Pipeline execution failed",
			zap.Error(err),
			zap.Bool("fallback_mode", s.fallbackMode))

		// 如果启用 fallback 模式且检索失败，返回空结果
		if s.fallbackMode && state.RetrievedChunks == nil {
			state.FinalResult = &domain.ReviewResult{
				OverallResult: "pass",
				RiskScore:     0,
				Issues:        []domain.Issue{},
			}
		} else {
			return nil, fmt.Errorf("review pipeline failed: %w", err)
		}
	}

	logger.Log.Info("Case review completed",
		zap.String("request_id", req.RequestID),
		zap.String("overall_result", state.FinalResult.OverallResult),
		zap.Int("risk_score", state.FinalResult.RiskScore),
		zap.Int("issue_count", len(state.FinalResult.Issues)))

	return state.FinalResult, nil
}

func (s *ReviewService) assembleCaseContent(docs []domain.Document) string {
	var builder strings.Builder

	for _, doc := range docs {
		builder.WriteString(fmt.Sprintf("【文档：%s】\n", doc.Filename))
		builder.WriteString(doc.Content)
		builder.WriteString("\n\n")
	}

	return builder.String()
}

// formatVector 格式化向量为字符串（使用优化版本）
func formatVector(vec []float32) string {
	return utils.FormatVectorWithoutCache(vec)
}
