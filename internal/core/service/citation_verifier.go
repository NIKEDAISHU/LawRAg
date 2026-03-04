package service

import (
	"context"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"

	"go.uber.org/zap"
)

// LawCitationVerifier 法律引用验证器
// 负责校验 LLM 输出的法律引用是否存在于检索结果中
type LawCitationVerifier struct {
	lawRepo port.LawRepository
}

func NewLawCitationVerifier(lawRepo port.LawRepository) *LawCitationVerifier {
	return &LawCitationVerifier{lawRepo: lawRepo}
}

// VerifyCitations 校验法律引用的有效性
// 返回验证后的引用列表，标记 verified 字段
func (v *LawCitationVerifier) VerifyCitations(ctx context.Context, citations []domain.LawCitation, retrievedChunks []domain.LawChunk) []domain.LawCitation {
	if len(citations) == 0 {
		return citations
	}

	// 构建检索结果的内容索引
	contentMap := make(map[string]string)
	for _, chunk := range retrievedChunks {
		lawName, _ := chunk.Metadata["law_name"].(string)
		articleID, _ := chunk.Metadata["article_id"].(string)
		if lawName != "" {
			contentMap[lawName] = chunk.Content
		}
		if articleID != "" && lawName != "" {
			key := lawName + "_" + articleID
			contentMap[key] = chunk.Content
		}
	}

	// 验证每个引用
	for i := range citations {
		citation := &citations[i]

		// 检查法规名称是否匹配
		matched := false
		for lawName, content := range contentMap {
			if containsLawName(lawName, citation.LawID) {
				// 检查条款是否匹配
				if containsArticle(content, citation.Article) {
					matched = true
					citation.Verified = true
					citation.Content = extractArticleContent(content, citation.Article)
					break
				}
			}
		}

		if !matched {
			citation.Verified = false
			logger.Log.Warn("Citation not verified",
				zap.String("law_id", citation.LawID),
				zap.String("article", citation.Article))
		}
	}

	return citations
}

// VerifyCitationsFromState 从评审状态中校验引用
func (v *LawCitationVerifier) VerifyCitationsFromState(ctx context.Context, state *port.ReviewState) {
	if state.ValidationResult == nil || len(state.ValidationResult.Issues) == 0 {
		return
	}

	// 收集所有引用
	var allCitations []domain.LawCitation
	for _, issue := range state.ValidationResult.Issues {
		allCitations = append(allCitations, issue.Citations...)
	}

	// 校验
	verifiedCitations := v.VerifyCitations(ctx, allCitations, state.RetrievedChunks)

	// 更新 Issues 中的引用
	citationIndex := 0
	for i := range state.ValidationResult.Issues {
		issue := &state.ValidationResult.Issues[i]
		count := len(issue.Citations)
		if count > 0 {
			issue.Citations = verifiedCitations[citationIndex : citationIndex+count]
			citationIndex += count
		}
	}
}

// containsLawName 检查法规名称是否包含关键词
func containsLawName(lawName, keyword string) bool {
	if lawName == "" || keyword == "" {
		return false
	}
	// 简单匹配：法规名称包含关键词
	return len(keyword) >= 2 && (lawName == keyword ||
		contains(keyword, lawName) ||
		contains(lawName, keyword))
}

// containsArticle 检查内容中是否包含条款
func containsArticle(content, article string) bool {
	if content == "" || article == "" {
		return false
	}
	return contains(content, article)
}

// extractArticleContent 提取条款的具体内容
func extractArticleContent(content, article string) string {
	// 简单实现：返回原文
	return content
}

// contains 字符串包含判断
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
