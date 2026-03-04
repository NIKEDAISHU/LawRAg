package service

import (
	"context"
	"encoding/json"
	"fmt"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"
	"strings"

	"go.uber.org/zap"
)

// ExtractInfoStep 信息提取步骤
type ExtractInfoStep struct {
	llmClient port.CaseReviewLLM
}

func NewExtractInfoStep(llmClient port.CaseReviewLLM) *ExtractInfoStep {
	return &ExtractInfoStep{llmClient: llmClient}
}

func (s *ExtractInfoStep) Name() string {
	return "ExtractInfo"
}

func (s *ExtractInfoStep) Skip(ctx context.Context, state *port.ReviewState) bool {
	return state.CaseContent == ""
}

func (s *ExtractInfoStep) Execute(ctx context.Context, state *port.ReviewState) error {
	logger.Log.Info("Executing ExtractInfo step",
		zap.String("case_content_length", fmt.Sprintf("%d", len(state.CaseContent))))

	response, err := s.llmClient.ExtractInfo(ctx, state.CaseContent)
	if err != nil {
		return fmt.Errorf("failed to extract info: %w", err)
	}

	var extractedInfo domain.ExtractedInfo
	if err := json.Unmarshal([]byte(response), &extractedInfo); err != nil {
		return fmt.Errorf("failed to parse extracted info: %w, response: %s", err, response)
	}

	state.ExtractedInfo = &extractedInfo
	logger.Log.Info("Extracted key information",
		zap.String("violation_facts", extractedInfo.ViolationFacts),
		zap.Strings("cited_laws", extractedInfo.CitedLaws))

	return nil
}

// RetrieveLawsStep 检索相关法规步骤
type RetrieveLawsStep struct {
	strategy port.RetrievalStrategy
}

func NewRetrieveLawsStep(strategy port.RetrievalStrategy) *RetrieveLawsStep {
	return &RetrieveLawsStep{strategy: strategy}
}

func (s *RetrieveLawsStep) Name() string {
	return "RetrieveLaws"
}

func (s *RetrieveLawsStep) Skip(ctx context.Context, state *port.ReviewState) bool {
	return state.ExtractedInfo == nil
}

func (s *RetrieveLawsStep) Execute(ctx context.Context, state *port.ReviewState) error {
	logger.Log.Info("Executing RetrieveLaws step",
		zap.String("strategy", s.strategy.Name()))

	chunks, err := s.strategy.RetrieveDirect(ctx, state.CaseContent, state.ProjectIDs)
	if err != nil {
		return fmt.Errorf("failed to retrieve laws: %w", err)
	}

	state.RetrievedChunks = chunks

	// 格式化检索结果
	var builder strings.Builder
	for i, chunk := range chunks {
		builder.WriteString(fmt.Sprintf("【相关法规 %d】\n", i+1))
		if lawName, ok := chunk.Metadata["law_name"].(string); ok && lawName != "" {
			builder.WriteString(fmt.Sprintf("法规名称：%s\n", lawName))
		}
		if articleID, ok := chunk.Metadata["article_id"].(string); ok && articleID != "" {
			builder.WriteString(fmt.Sprintf("条款：%s\n", articleID))
		}
		builder.WriteString(fmt.Sprintf("内容：%s\n\n", chunk.Content))
	}

	state.RetrievedLaws = builder.String()

	logger.Log.Info("Retrieved relevant laws",
		zap.Int("chunk_count", len(chunks)))

	return nil
}

// ValidateLawsStep 法律验证步骤
type ValidateLawsStep struct {
	llmClient port.CaseReviewLLM
}

func NewValidateLawsStep(llmClient port.CaseReviewLLM) *ValidateLawsStep {
	return &ValidateLawsStep{llmClient: llmClient}
}

func (s *ValidateLawsStep) Name() string {
	return "ValidateLaws"
}

func (s *ValidateLawsStep) Skip(ctx context.Context, state *port.ReviewState) bool {
	// 当有提取信息或案件内容时，都执行验证
	return state.ExtractedInfo == nil && state.CaseContent == ""
}

func (s *ValidateLawsStep) Execute(ctx context.Context, state *port.ReviewState) error {
	var facts, citedLaws string

	if state.ExtractedInfo != nil {
		// 使用提取的信息
		facts = state.ExtractedInfo.ViolationFacts
		citedLaws = strings.Join(state.ExtractedInfo.CitedLaws, "\n")
	} else {
		// 直接使用案件内容（简化流程模式）
		facts = state.CaseContent
		citedLaws = ""
	}

	response, err := s.llmClient.ValidateLaws(ctx, facts, citedLaws, state.RetrievedLaws)
	if err != nil {
		return fmt.Errorf("failed to validate laws: %w", err)
	}

	var validationResult domain.ValidationResult
	if err := json.Unmarshal([]byte(response), &validationResult); err != nil {
		return fmt.Errorf("failed to parse validation result: %w, response: %s", err, response)
	}

	state.ValidationResult = &validationResult

	logger.Log.Info("Validation completed",
		zap.Bool("is_correct", validationResult.IsCorrect),
		zap.Int("risk_score", validationResult.RiskScore))

	return nil
}

// VerifyCitationsStep 法律引用验证步骤
type VerifyCitationsStep struct {
	verifier *LawCitationVerifier
}

func NewVerifyCitationsStep(verifier *LawCitationVerifier) *VerifyCitationsStep {
	return &VerifyCitationsStep{verifier: verifier}
}

func (s *VerifyCitationsStep) Name() string {
	return "VerifyCitations"
}

func (s *VerifyCitationsStep) Skip(ctx context.Context, state *port.ReviewState) bool {
	return state.ValidationResult == nil || len(state.RetrievedChunks) == 0
}

func (s *VerifyCitationsStep) Execute(ctx context.Context, state *port.ReviewState) error {
	s.verifier.VerifyCitationsFromState(ctx, state)

	// 统计验证结果
	verifiedCount := 0
	totalCount := 0
	for _, issue := range state.ValidationResult.Issues {
		for _, citation := range issue.Citations {
			totalCount++
			if citation.Verified {
				verifiedCount++
			}
		}
	}

	logger.Log.Info("Citation verification completed",
		zap.Int("verified", verifiedCount),
		zap.Int("total", totalCount))

	return nil
}

// AssembleResultStep 组装最终结果步骤
type AssembleResultStep struct{}

func NewAssembleResultStep() *AssembleResultStep {
	return &AssembleResultStep{}
}

func (s *AssembleResultStep) Name() string {
	return "AssembleResult"
}

func (s *AssembleResultStep) Skip(ctx context.Context, state *port.ReviewState) bool {
	return state.ValidationResult == nil
}

func (s *AssembleResultStep) Execute(ctx context.Context, state *port.ReviewState) error {
	validation := state.ValidationResult

	overallResult := "pass"
	if !validation.IsCorrect || validation.RiskScore > 50 {
		overallResult = "fail"
	}

	// 填充相关文档信息
	for i := range validation.Issues {
		if validation.Issues[i].RelatedDoc == "" && len(state.DocList) > 0 {
			validation.Issues[i].RelatedDoc = state.DocList[0].Filename
		}
	}

	state.FinalResult = &domain.ReviewResult{
		OverallResult: overallResult,
		RiskScore:     validation.RiskScore,
		Issues:        validation.Issues,
	}

	logger.Log.Info("Result assembled",
		zap.String("overall_result", overallResult),
		zap.Int("risk_score", validation.RiskScore),
		zap.Int("issue_count", len(validation.Issues)))

	return nil
}

// extractKeywords 从提取的信息中提取关键词
func extractKeywords(info *domain.ExtractedInfo) []string {
	keywords := make([]string, 0)

	for _, law := range info.CitedLaws {
		if strings.Contains(law, "第") && strings.Contains(law, "条") {
			keywords = append(keywords, law)
		}
	}

	for _, fact := range info.KeyFacts {
		if len(fact) > 2 && len(fact) < 20 {
			keywords = append(keywords, fact)
		}
	}

	return keywords
}
