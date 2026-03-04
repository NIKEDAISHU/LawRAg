package utils

import (
	"fmt"
	"strings"
)

// ExtractKeywords 从文本中提取关键词
// 支持中文和英文，过滤停用词
func ExtractKeywords(text string, minLength int) []string {
	keywords := []string{}
	var word []rune

	// 按标点符号拆分
	for _, ch := range text {
		if ch == ' ' || ch == ',' || ch == '，' || ch == '、' ||
			ch == '。' || ch == '？' || ch == '?' ||
			ch == '\n' || ch == '\t' {
			if len(word) > 0 {
				keywords = append(keywords, string(word))
				word = nil
			}
		} else {
			word = append(word, ch)
		}
	}
	if len(word) > 0 {
		keywords = append(keywords, string(word))
	}

	// 停用词
	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true, "有": true,
		"和": true, "与": true, "或": true, "等": true, "吗": true,
		"什么": true, "哪些": true, "如何": true, "怎样": true,
		"请": true, "根据": true, "按照": true, "关于": true,
	}

	// 过滤
	result := []string{}
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if len(kw) >= minLength && !stopWords[kw] {
			result = append(result, kw)
		}
	}

	return result
}

// FormatVector 格式化向量为字符串
func FormatVector(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	strVec := make([]string, len(vec))
	for i, v := range vec {
		strVec[i] = formatFloat32(v)
	}
	return "[" + strings.Join(strVec, ",") + "]"
}

func formatFloat32(f float32) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprint(float64(f)), "0"), ".")
}
