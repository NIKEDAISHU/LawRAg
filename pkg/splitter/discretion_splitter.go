package splitter

import (
	"regexp"
	"strings"

	"law-enforcement-brain/internal/core/domain"
)

// DiscretionSplitter 裁量基准文档切分器
// 按 ====== 分割文档，提取结构化字段
type DiscretionSplitter struct {
	regionPattern     *regexp.Regexp
	violationPattern  *regexp.Regexp
	legalBasisPattern *regexp.Regexp
	severityPattern   *regexp.Regexp
	conditionsPattern *regexp.Regexp
	penaltyPattern    *regexp.Regexp
}

func NewDiscretionSplitter() *DiscretionSplitter {
	return &DiscretionSplitter{
		regionPattern:     regexp.MustCompile(`【([^】]+省|[^】]+市|[^】]+自治区)裁量基准】`),
		violationPattern:  regexp.MustCompile(`【违法行为】[：:]\s*(.+?)(?:\n|$)`),
		legalBasisPattern: regexp.MustCompile(`【法律依据】[：:]\s*(.+?)(?:\n【|$)`),
		severityPattern:   regexp.MustCompile(`【违法程度】[：:]\s*(.+?)(?:\n|$)`),
		conditionsPattern: regexp.MustCompile(`【适用情形】[：:]\s*(.+?)(?:\n【|$)`),
		penaltyPattern:    regexp.MustCompile(`【处罚标准】[：:]\s*(.+?)(?:\n|$)`),
	}
}

// Split 切分裁量基准文档
func (s *DiscretionSplitter) Split(content string) []domain.LawChunk {
	var chunks []domain.LawChunk

	// 按 ====== 分割文档
	sections := strings.Split(content, "======")

	for i, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}

		// 提取结构化字段
		region := s.extractField(section, s.regionPattern)
		violation := s.extractField(section, s.violationPattern)
		legalBasis := s.extractField(section, s.legalBasisPattern)
		severity := s.extractField(section, s.severityPattern)
		conditions := s.extractField(section, s.conditionsPattern)
		penalty := s.extractField(section, s.penaltyPattern)

		// 构建 metadata
		metadata := map[string]interface{}{
			"chunk_index": i,
			"doc_type":    "discretion_standard",
			"region":      region,
			"violation":   violation,
			"severity":    severity,
		}

		// 添加可选字段
		if legalBasis != "" {
			metadata["legal_basis"] = legalBasis
		}
		if conditions != "" {
			metadata["conditions"] = conditions
		}
		if penalty != "" {
			metadata["penalty"] = penalty
		}

		// 提取法律名称（从法律依据中）
		lawName := s.extractLawName(legalBasis)
		if lawName != "" {
			metadata["law_name"] = lawName
		}

		chunks = append(chunks, domain.LawChunk{
			Content:  section,
			Metadata: metadata,
		})
	}

	return chunks
}

// extractField 提取字段内容
func (s *DiscretionSplitter) extractField(content string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractLawName 从法律依据中提取法律名称
func (s *DiscretionSplitter) extractLawName(legalBasis string) string {
	// 匹配《法律名称》
	lawPattern := regexp.MustCompile(`《([^》]+)》`)
	matches := lawPattern.FindStringSubmatch(legalBasis)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}
