package utils

import (
	"strconv"
	"strings"
	"sync"
)

// VectorCache 缓存已格式化的向量字符串
type VectorCache struct {
	mu    sync.RWMutex
	cache map[string]string
}

// NewVectorCache 创建新的向量缓存
func NewVectorCache() *VectorCache {
	return &VectorCache{
		cache: make(map[string]string),
	}
}

// FormatVector 格式化向量为字符串（带缓存）
// 使用 strconv 替代 fmt.Sprintf 提高性能
func (vc *VectorCache) FormatVector(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}

	// 尝试从缓存读取
	key := vectorKey(vec)
	vc.mu.RLock()
	if cached, ok := vc.cache[key]; ok {
		vc.mu.RUnlock()
		return cached
	}
	vc.mu.RUnlock()

	// 格式化向量
	strVec := make([]string, len(vec))
	for i, v := range vec {
		// 使用 strconv.FormatFloat 比 fmt.Sprintf 快约 3-5 倍
		strVec[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	result := "[" + strings.Join(strVec, ",") + "]"

	// 存入缓存
	vc.mu.Lock()
	vc.cache[key] = result
	vc.mu.Unlock()

	return result
}

// FormatVectorWithoutCache 格式化向量（无缓存版本，适用于一次性转换）
func FormatVectorWithoutCache(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}

	strVec := make([]string, len(vec))
	for i, v := range vec {
		strVec[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return "[" + strings.Join(strVec, ",") + "]"
}

// vectorKey 生成向量的缓存键
// 使用向量的部分值作为键，避免内存占用过大
func vectorKey(vec []float32) string {
	// 对于向量，我们使用前8个值作为键
	// 这样可以在大部分情况下保持唯一性，同时减少内存占用
	const maxKeyElements = 8
	n := len(vec)
	if n > maxKeyElements {
		n = maxKeyElements
	}

	keyParts := make([]string, n)
	for i := 0; i < n; i++ {
		keyParts[i] = strconv.FormatFloat(float64(vec[i]), 'f', -1, 32)
	}
	return strings.Join(keyParts, ",")
}

// Clear 清空缓存
func (vc *VectorCache) Clear() {
	vc.mu.Lock()
	vc.cache = make(map[string]string)
	vc.mu.Unlock()
}

// Size 返回缓存大小
func (vc *VectorCache) Size() int {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return len(vc.cache)
}
