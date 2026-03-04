package splitter

import (
	"regexp"
	"strings"

	"law-enforcement-brain/internal/core/domain"
)

// MdSplitter Markdown 格式法律文档切分器
// 按 --- 分割文档，提取【法规名】和第X条
type MdSplitter struct {
	articlePattern *regexp.Regexp
	lawNamePattern *regexp.Regexp
}

func NewMdSplitter() *MdSplitter {
	return &MdSplitter{
		articlePattern: regexp.MustCompile(`第([零一二三四五六七八九十百千万\d]+)条`),
		lawNamePattern: regexp.MustCompile(`【法律名称】(.+?)(第[零一二三四五六七八九十百千万\d]+条)`),
	}
}

// Split 切分 Markdown 格式的法律文档
func (s *MdSplitter) Split(content string) []domain.LawChunk {
	var chunks []domain.LawChunk

	// 按 --- 分割文档
	sections := strings.Split(content, "---")

	// 提取法规名（从第一个 section 或整个文档中）
	lawName := s.extractLawName(content)

	for i, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}

		// 提取条款号
		articleIndex, articleID := s.extractArticleIndex(section)

		// 构建 metadata
		metadata := map[string]interface{}{
			"law_name":      lawName,
			"chunk_index":   i,
			"section_index": i,
		}

		if articleIndex > 0 {
			metadata["article_index"] = articleIndex
			metadata["article_id"] = articleID
		}

		chunks = append(chunks, domain.LawChunk{
			Content:  section,
			Metadata: metadata,
		})
	}

	return chunks
}

// extractLawName 提取【法规名】
func (s *MdSplitter) extractLawName(content string) string {
	matches := s.lawNamePattern.FindStringSubmatch(content)
	if len(matches) > 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractArticleIndex 提取条款号（第X条）
func (s *MdSplitter) extractArticleIndex(content string) (int, string) {
	matches := s.articlePattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		articleID := matches[0] // 完整的"第X条"
		chineseNum := matches[1]

		// 转换为阿拉伯数字
		arabicNum := s.chineseToArabic(chineseNum)
		return arabicNum, articleID
	}
	return 0, ""
}

// chineseToArabic 中文数字转阿拉伯数字
func (s *MdSplitter) chineseToArabic(chinese string) int {
	// 如果已经是阿拉伯数字，直接返回
	if len(chinese) > 0 && chinese[0] >= '0' && chinese[0] <= '9' {
		num := 0
		for _, ch := range chinese {
			if ch >= '0' && ch <= '9' {
				num = num*10 + int(ch-'0')
			}
		}
		return num
	}

	// 中文数字映射
	digitMap := map[rune]int{
		'零': 0, '一': 1, '二': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
	}

	unitMap := map[rune]int{
		'十': 10, '百': 100, '千': 1000, '万': 10000,
	}

	result := 0
	section := 0

	runes := []rune(chinese)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		if val, ok := digitMap[ch]; ok {
			section = section*10 + val
		} else if val, ok := unitMap[ch]; ok {
			if section == 0 {
				section = 1
			}
			if val >= 10000 {
				result = (result + section) * val
				section = 0
			} else {
				result += section * val
				section = 0
			}
		}
	}

	result += section
	return result
}
