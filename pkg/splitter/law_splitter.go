package splitter

import (
	"regexp"
	"strings"

	"law-enforcement-brain/internal/core/domain"
)

type LawSplitter struct {
	articlePattern *regexp.Regexp
	lawNamePattern *regexp.Regexp
}

func NewLawSplitter() *LawSplitter {
	return &LawSplitter{
		articlePattern: regexp.MustCompile(`第[一二三四五六七八九十百千]+条`),
		lawNamePattern: regexp.MustCompile(`《([^》]+)》`),
	}
}

func (s *LawSplitter) Split(text string) []domain.LawChunk {
	var chunks []domain.LawChunk

	lawName := s.extractLawName(text)

	indices := s.articlePattern.FindAllStringIndex(text, -1)

	if len(indices) == 0 {
		if strings.TrimSpace(text) != "" {
			chunks = append(chunks, domain.LawChunk{
				Content: strings.TrimSpace(text),
				Metadata: map[string]interface{}{
					"law_name":   lawName,
					"article_id": "",
				},
			})
		}
		return chunks
	}

	for i := 0; i < len(indices); i++ {
		start := indices[i][0]
		var end int
		if i+1 < len(indices) {
			end = indices[i+1][0]
		} else {
			end = len(text)
		}

		chunkText := strings.TrimSpace(text[start:end])
		if chunkText == "" {
			continue
		}

		articleID := s.extractArticleID(chunkText)
		articleIndex := s.extractArticleIndex(articleID)

		chunks = append(chunks, domain.LawChunk{
			Content: chunkText,
			Metadata: map[string]interface{}{
				"law_name":      lawName,
				"article_id":    articleID,
				"article_index": articleIndex, // 阿拉伯数字索引，用于精确查询
			},
		})
	}

	return chunks
}

func (s *LawSplitter) extractLawName(text string) string {
	matches := s.lawNamePattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func (s *LawSplitter) extractArticleID(text string) string {
	match := s.articlePattern.FindString(text)
	return match
}

// extractArticleIndex 从条款文本中提取阿拉伯数字索引
// 例如："第八条" -> 8, "第二十三条" -> 23
func (s *LawSplitter) extractArticleIndex(articleID string) int {
	if articleID == "" {
		return 0
	}

	// 提取中文数字部分（去掉"第"和"条"）
	chineseNum := strings.TrimPrefix(articleID, "第")
	chineseNum = strings.TrimSuffix(chineseNum, "条")

	// 转换为阿拉伯数字
	return ChineseToArabic(chineseNum)
}

func (s *LawSplitter) SplitWithLawName(text string, lawName string) []domain.LawChunk {
	chunks := s.Split(text)
	for i := range chunks {
		if chunks[i].Metadata["law_name"] == "" {
			chunks[i].Metadata["law_name"] = lawName
		}
	}
	return chunks
}

var chineseNumerals = map[rune]int{
	'零': 0, '一': 1, '二': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
	'十': 10, '百': 100, '千': 1000,
}

func ChineseToArabic(chinese string) int {
	if chinese == "" {
		return 0
	}

	runes := []rune(chinese)
	result := 0
	temp := 0

	for i := 0; i < len(runes); i++ {
		val, exists := chineseNumerals[runes[i]]
		if !exists {
			continue
		}

		if val >= 10 {
			if temp == 0 {
				temp = 1
			}
			result += temp * val
			temp = 0
		} else {
			temp = val
		}
	}

	result += temp
	return result
}
