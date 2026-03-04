package service

import (
	"context"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"
	"strings"

	"go.uber.org/zap"
)

// HybridRetrievalStrategy 混合检索策略
type HybridRetrievalStrategy struct {
	lawRepo    port.LawRepository
	embedding  port.EmbeddingLLM
	projectIDs []int64
	topK       int
}

func NewHybridRetrievalStrategy(lawRepo port.LawRepository, embedding port.EmbeddingLLM, projectIDs []int64, topK int) *HybridRetrievalStrategy {
	return &HybridRetrievalStrategy{
		lawRepo:    lawRepo,
		embedding:  embedding,
		projectIDs: projectIDs,
		topK:       topK,
	}
}

func (s *HybridRetrievalStrategy) Name() string {
	return "hybrid"
}

func (s *HybridRetrievalStrategy) Retrieve(ctx context.Context, state *port.ReviewState) ([]domain.LawChunk, error) {
	// 构建查询文本
	queryText := buildQueryText(state.ExtractedInfo)

	// 创建向量
	vector, err := s.embedding.CreateEmbedding(ctx, queryText)
	if err != nil {
		return nil, err
	}
	vectorStr := formatVector(vector)

	// 提取关键词
	keywords := extractKeywordsForRetrieval(state.ExtractedInfo)

	// 混合检索
	chunks, err := s.lawRepo.HybridSearch(ctx, s.projectIDs, vectorStr, keywords, s.topK)
	if err != nil {
		logger.Log.Error("Hybrid search failed", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("Hybrid retrieval completed", zap.Int("chunks", len(chunks)))
	return chunks, nil
}

// RetrieveDirect 直接使用查询文本检索（跳过 ExtractInfo）
func (s *HybridRetrievalStrategy) RetrieveDirect(ctx context.Context, query string, projectIDs []int64) ([]domain.LawChunk, error) {
	if query == "" {
		return nil, nil
	}

	// 创建向量
	vector, err := s.embedding.CreateEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}
	vectorStr := formatVector(vector)

	// 简单关键词提取
	keywords := extractSimpleKeywords(query)

	// 混合检索
	chunks, err := s.lawRepo.HybridSearch(ctx, projectIDs, vectorStr, keywords, s.topK)
	if err != nil {
		logger.Log.Error("Hybrid search failed", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("Hybrid retrieval completed", zap.Int("chunks", len(chunks)))
	return chunks, nil
}

// VectorRetrievalStrategy 向量检索策略
type VectorRetrievalStrategy struct {
	lawRepo    port.LawRepository
	embedding  port.EmbeddingLLM
	projectIDs []int64
	topK       int
}

func NewVectorRetrievalStrategy(lawRepo port.LawRepository, embedding port.EmbeddingLLM, projectIDs []int64, topK int) *VectorRetrievalStrategy {
	return &VectorRetrievalStrategy{
		lawRepo:    lawRepo,
		embedding:  embedding,
		projectIDs: projectIDs,
		topK:       topK,
	}
}

func (s *VectorRetrievalStrategy) Name() string {
	return "vector"
}

func (s *VectorRetrievalStrategy) Retrieve(ctx context.Context, state *port.ReviewState) ([]domain.LawChunk, error) {
	queryText := buildQueryText(state.ExtractedInfo)

	vector, err := s.embedding.CreateEmbedding(ctx, queryText)
	if err != nil {
		return nil, err
	}

	chunks, err := s.lawRepo.SearchSimilar(ctx, s.projectIDs, formatVector(vector), s.topK)
	if err != nil {
		logger.Log.Error("Vector search failed", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("Vector retrieval completed", zap.Int("chunks", len(chunks)))
	return chunks, nil
}

// RetrieveDirect 直接使用查询文本检索
func (s *VectorRetrievalStrategy) RetrieveDirect(ctx context.Context, query string, projectIDs []int64) ([]domain.LawChunk, error) {
	if query == "" {
		return nil, nil
	}

	vector, err := s.embedding.CreateEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}

	chunks, err := s.lawRepo.SearchSimilar(ctx, projectIDs, formatVector(vector), s.topK)
	if err != nil {
		logger.Log.Error("Vector search failed", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("Vector retrieval completed", zap.Int("chunks", len(chunks)))
	return chunks, nil
}

// KeywordRetrievalStrategy 关键词检索策略
type KeywordRetrievalStrategy struct {
	lawRepo    port.LawRepository
	projectIDs []int64
	topK       int
}

func NewKeywordRetrievalStrategy(lawRepo port.LawRepository, projectIDs []int64, topK int) *KeywordRetrievalStrategy {
	return &KeywordRetrievalStrategy{
		lawRepo:    lawRepo,
		projectIDs: projectIDs,
		topK:       topK,
	}
}

func (s *KeywordRetrievalStrategy) Name() string {
	return "keyword"
}

func (s *KeywordRetrievalStrategy) Retrieve(ctx context.Context, state *port.ReviewState) ([]domain.LawChunk, error) {
	keywords := extractKeywordsForRetrieval(state.ExtractedInfo)

	chunks, err := s.lawRepo.SearchByKeywords(ctx, s.projectIDs, keywords)
	if err != nil {
		logger.Log.Error("Keyword search failed", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("Keyword retrieval completed", zap.Int("chunks", len(chunks)))
	return chunks, nil
}

// RetrieveDirect 直接使用查询文本检索
func (s *KeywordRetrievalStrategy) RetrieveDirect(ctx context.Context, query string, projectIDs []int64) ([]domain.LawChunk, error) {
	if query == "" {
		return nil, nil
	}

	keywords := extractSimpleKeywords(query)

	chunks, err := s.lawRepo.SearchByKeywords(ctx, projectIDs, keywords)
	if err != nil {
		logger.Log.Error("Keyword search failed", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("Keyword retrieval completed", zap.Int("chunks", len(chunks)))
	return chunks, nil
}

// AdaptiveRetrievalStrategy 自适应检索策略
// 根据查询特征自动选择检索策略
type AdaptiveRetrievalStrategy struct {
	hybrid  port.RetrievalStrategy
	vector  port.RetrievalStrategy
	keyword port.RetrievalStrategy
}

func NewAdaptiveRetrievalStrategy(hybrid, vector, keyword port.RetrievalStrategy) *AdaptiveRetrievalStrategy {
	return &AdaptiveRetrievalStrategy{
		hybrid:  hybrid,
		vector:  vector,
		keyword: keyword,
	}
}

func (s *AdaptiveRetrievalStrategy) Name() string {
	return "adaptive"
}

func (s *AdaptiveRetrievalStrategy) Retrieve(ctx context.Context, state *port.ReviewState) ([]domain.LawChunk, error) {
	queryText := buildQueryText(state.ExtractedInfo)

	// 自适应选择策略
	strategy := s.selectStrategy(queryText)
	logger.Log.Info("Adaptive strategy selected", zap.String("strategy", strategy.Name()))

	return strategy.Retrieve(ctx, state)
}

// RetrieveDirect 直接使用查询文本检索
func (s *AdaptiveRetrievalStrategy) RetrieveDirect(ctx context.Context, query string, projectIDs []int64) ([]domain.LawChunk, error) {
	if query == "" {
		return nil, nil
	}

	// 自适应选择策略
	strategy := s.selectStrategy(query)
	logger.Log.Info("Adaptive strategy selected", zap.String("strategy", strategy.Name()))

	return strategy.RetrieveDirect(ctx, query, projectIDs)
}

// selectStrategy 根据查询特征选择策略
func (s *AdaptiveRetrievalStrategy) selectStrategy(query string) port.RetrievalStrategy {
	// 短文本：优先关键词
	if len(query) < 50 {
		return s.keyword
	}

	// 包含法条编号：关键词优先
	if containsLawArticleNumber(query) {
		return s.keyword
	}

	// 长文本：混合检索
	return s.hybrid
}

// buildQueryText 构建查询文本
func buildQueryText(info *domain.ExtractedInfo) string {
	if info.ViolationFacts != "" {
		return info.ViolationFacts
	}
	if len(info.KeyFacts) > 0 {
		return strings.Join(info.KeyFacts, " ")
	}
	return ""
}

// extractKeywordsForRetrieval 提取检索关键词
func extractKeywordsForRetrieval(info *domain.ExtractedInfo) []string {
	keywords := make([]string, 0)

	// 从引用法条中提取
	for _, law := range info.CitedLaws {
		if strings.Contains(law, "第") && strings.Contains(law, "条") {
			keywords = append(keywords, law)
		}
	}

	// 从关键事实中提取
	for _, fact := range info.KeyFacts {
		if len(fact) > 2 && len(fact) < 20 {
			keywords = append(keywords, fact)
		}
	}

	return keywords
}

// containsLawArticleNumber 检查是否包含法条编号
func containsLawArticleNumber(text string) bool {
	// 简单检测：包含"第"和"条"且中间有数字
	return strings.Contains(text, "第") && strings.Contains(text, "条")
}

// extractSimpleKeywords 从文本中提取简单关键词
func extractSimpleKeywords(text string) []string {
	keywords := make([]string, 0)
	runes := []rune(text)

	// 提取法条引用
	for i := 0; i < len(runes)-2; i++ {
		if runes[i] == '第' {
			// 找到"第"后，提取到"条"为止
			end := i + 1
			for end < len(runes) && runes[end] != '条' && end < i+20 {
				end++
			}
			if end < len(runes) && runes[end] == '条' {
				law := string(runes[i : end+1])
				if len(law) > 2 && len(law) < 30 {
					keywords = append(keywords, law)
				}
			}
		}
	}

	return keywords
}
