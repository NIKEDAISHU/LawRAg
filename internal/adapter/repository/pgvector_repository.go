package repository

import (
	"context"
	"errors"
	"fmt"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/pkg/config"
	"law-enforcement-brain/pkg/logger"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type KnowledgeChunk struct {
	ID           int64                  `gorm:"primaryKey;column:id"`
	ProjectID    int64                  `gorm:"column:project_id;not null;index"`
	DocName      string                 `gorm:"column:doc_name;type:varchar(255)"`
	ChunkContent string                 `gorm:"column:chunk_content;type:text"`
	ChunkIndex   int                    `gorm:"column:chunk_index"`
	Embedding    pgvector.Vector        `gorm:"column:embedding;type:vector(1024)"`
	QAContent    string                 `gorm:"column:qa_content;type:text"`           // QA 对内容
	QAEmbedding  *pgvector.Vector       `gorm:"column:qa_embedding;type:vector(1024)"` // QA 向量（可为空）
	Metadata     map[string]interface{} `gorm:"column:metadata;type:jsonb;serializer:json"`
	CreatedAt    time.Time              `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time              `gorm:"column:updated_at;autoUpdateTime"`
}

func (KnowledgeChunk) TableName() string {
	return "knowledge_chunks"
}

type PgVectorRepository struct {
	db *gorm.DB
}

func NewPgVectorRepository(cfg *config.DatabaseConfig) (*PgVectorRepository, *gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return nil, nil, fmt.Errorf("failed to create vector extension: %w", err)
	}

	// 配置数据库连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// SetMaxIdleConns 设置空闲连接池中连接的最大数量
	// 建议值：根据应用的并发量，通常 10-20 是合理的
	sqlDB.SetMaxIdleConns(10)

	// SetMaxOpenConns 设置打开数据库连接的最大数量
	// 建议值：根据应用并发需求和数据库服务器能力，通常 50-100 是合理的
	sqlDB.SetMaxOpenConns(100)

	// SetConnMaxLifetime 设置连接可复用的最长时间
	// 建议值：通常设置为 30 分钟到 1 小时，避免长时间连接的问题
	sqlDB.SetConnMaxLifetime(time.Hour)

	// SetConnMaxIdleTime 设置连接可能空闲的最长时间
	// 建议值：10-30 分钟，及时关闭不用的连接
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	logger.Log.Info("Database connection pool configured",
		zap.Int("max_idle_conns", 10),
		zap.Int("max_open_conns", 100),
		zap.Duration("conn_max_lifetime", time.Hour),
		zap.Duration("conn_max_idle_time", 10*time.Minute))

	// 注意：表结构已通过迁移脚本创建，不再使用AutoMigrate
	// 如果需要修改表结构，请创建新的迁移脚本

	return &PgVectorRepository{db: db}, db, nil
}

func (r *PgVectorRepository) Store(ctx context.Context, projectID int64, content string, vector []float32, metadata map[string]string) error {
	return r.StoreWithQA(ctx, projectID, content, vector, "", nil, metadata)
}

// StoreWithQA 存储内容向量和 QA 向量
func (r *PgVectorRepository) StoreWithQA(ctx context.Context, projectID int64, content string, vector []float32, qaContent string, qaVector []float32, metadata map[string]string) error {
	docName := metadata["law_name"]
	if docName == "" {
		docName = metadata["filename"]
	}
	if docName == "" {
		docName = "未知文档"
	}

	chunkIndex := 0
	if idx, ok := metadata["chunk_index"]; ok {
		fmt.Sscanf(idx, "%d", &chunkIndex)
	}

	dbMetadata := make(map[string]interface{})
	dbMetadata["law_name"] = docName
	dbMetadata["chunk_index"] = chunkIndex

	if articleNo, ok := metadata["article_index"]; ok {
		dbMetadata["article_index"] = articleNo
	}
	if category, ok := metadata["category"]; ok {
		dbMetadata["category"] = category
	}
	if sourceURL, ok := metadata["source_url"]; ok {
		dbMetadata["source_url"] = sourceURL
	}

	chunk := KnowledgeChunk{
		ProjectID:    projectID,
		DocName:      docName,
		ChunkContent: content,
		ChunkIndex:   chunkIndex,
		Embedding:    pgvector.NewVector(vector),
		Metadata:     dbMetadata,
	}

	// 设置 QA 内容和向量
	if qaContent != "" && len(qaVector) > 0 {
		chunk.QAContent = qaContent
		v := pgvector.NewVector(qaVector)
		chunk.QAEmbedding = &v
	}

	if err := r.db.WithContext(ctx).Create(&chunk).Error; err != nil {
		return fmt.Errorf("failed to store chunk: %w", err)
	}

	return nil
}

func (r *PgVectorRepository) SearchSimilar(ctx context.Context, projectIDs []int64, queryVector string, topK int) ([]domain.LawChunk, error) {
	startTime := time.Now()

	var chunks []KnowledgeChunk

	query := r.db.WithContext(ctx)
	if len(projectIDs) > 0 {
		query = query.Where("project_id IN ?", projectIDs)
	}

	err := query.
		Order(fmt.Sprintf("embedding <=> '%s'", queryVector)).
		Limit(topK).
		Find(&chunks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to search similar: %w", err)
	}

	logger.Log.Info("Vector search completed",
		zap.Int("results", len(chunks)),
		zap.Int("topK", topK),
		zap.Duration("cost", time.Since(startTime)))

	return r.convertToDomainChunks(chunks), nil
}

func (r *PgVectorRepository) SearchByKeywords(ctx context.Context, projectIDs []int64, keywords []string) ([]domain.LawChunk, error) {
	startTime := time.Now()

	var chunks []KnowledgeChunk

	query := r.db.WithContext(ctx)
	if len(projectIDs) > 0 {
		query = query.Where("project_id IN ?", projectIDs)
	}

	for _, keyword := range keywords {
		query = query.Or("chunk_content LIKE ?", "%"+keyword+"%")
	}

	err := query.Limit(50).Find(&chunks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to search by keywords: %w", err)
	}

	logger.Log.Info("Keyword search completed",
		zap.Int("results", len(chunks)),
		zap.Strings("keywords", keywords),
		zap.Duration("cost", time.Since(startTime)))

	return r.convertToDomainChunks(chunks), nil
}

func (r *PgVectorRepository) HybridSearch(ctx context.Context, projectIDs []int64, queryVector string, keywords []string, topK int) ([]domain.LawChunk, error) {
	startTime := time.Now()

	// 安全验证：确保 queryVector 是有效的向量字符串格式
	if queryVector == "" {
		return nil, errors.New("query vector cannot be empty")
	}

	// 如果没有关键词，直接使用向量搜索
	if len(keywords) == 0 {
		return r.SearchSimilar(ctx, projectIDs, queryVector, topK)
	}

	// 清理关键词
	cleanedKeywords := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		cleaned := strings.TrimSpace(kw)
		if cleaned != "" {
			cleanedKeywords = append(cleanedKeywords, cleaned)
		}
	}

	if len(cleanedKeywords) == 0 {
		return r.SearchSimilar(ctx, projectIDs, queryVector, topK)
	}

	// 并行执行向量搜索和关键词搜索
	subLimit := topK * 3

	var vectorResults []domain.LawChunk
	var keywordResults []domain.LawChunk
	var vectorErr, keywordErr error

	var wg sync.WaitGroup
	wg.Add(2)

	// 向量搜索
	go func() {
		defer wg.Done()
		vectorResults, vectorErr = r.SearchSimilar(ctx, projectIDs, queryVector, subLimit)
	}()

	// 关键词搜索
	go func() {
		defer wg.Done()
		keywordResults, keywordErr = r.SearchByKeywords(ctx, projectIDs, cleanedKeywords)
	}()

	wg.Wait()

	// 处理向量搜索错误
	if vectorErr != nil {
		logger.Log.Warn("Vector search failed, using keyword search only", zap.Error(vectorErr))
		vectorResults = nil
	}

	// 处理关键词搜索错误
	if keywordErr != nil {
		logger.Log.Warn("Keyword search failed, using vector search only", zap.Error(keywordErr))
		keywordResults = nil
	}

	// 如果两者都失败，返回错误
	if vectorResults == nil && keywordResults == nil {
		return nil, fmt.Errorf("both vector and keyword search failed: vector=%w, keyword=%w", vectorErr, keywordErr)
	}

	// 使用 RRF (Reciprocal Rank Fusion) 合并结果
	const rrfK = 60
	results := r.rrfMerge(vectorResults, keywordResults, rrfK, topK)

	logger.Log.Info("Hybrid search completed",
		zap.Int("results", len(results)),
		zap.Strings("keywords", cleanedKeywords),
		zap.Duration("cost", time.Since(startTime)))

	return results, nil
}

// rrfMerge 使用 RRF 算法合并两个结果集
func (r *PgVectorRepository) rrfMerge(vectorResults, keywordResults []domain.LawChunk, rrfK, topK int) []domain.LawChunk {
	// 创建 map 存储每个 chunk 的得分
	scoreMap := make(map[int64]float64)
	chunkMap := make(map[int64]domain.LawChunk)

	// 为向量结果分配分数
	for rank, chunk := range vectorResults {
		score := 1.0 / float64(rrfK+rank+1)
		scoreMap[chunk.ID] += score
		chunkMap[chunk.ID] = chunk
	}

	// 为关键词结果分配分数
	for rank, chunk := range keywordResults {
		score := 1.0 / float64(rrfK+rank+1)
		scoreMap[chunk.ID] += score
		chunkMap[chunk.ID] = chunk
	}

	// 按分数排序
	type scoredChunk struct {
		id    int64
		score float64
	}
	var scoredChunks []scoredChunk
	for id, score := range scoreMap {
		scoredChunks = append(scoredChunks, scoredChunk{id: id, score: score})
	}

	// 排序（分数高的在前）
	sort.Slice(scoredChunks, func(i, j int) bool {
		return scoredChunks[i].score > scoredChunks[j].score
	})

	// 取 topK 结果
	if len(scoredChunks) > topK {
		scoredChunks = scoredChunks[:topK]
	}

	// 构建结果
	result := make([]domain.LawChunk, 0, len(scoredChunks))
	for _, sc := range scoredChunks {
		if chunk, ok := chunkMap[sc.id]; ok {
			result = append(result, chunk)
		}
	}

	return result
}

func (r *PgVectorRepository) InsertChunk(ctx context.Context, projectID int64, chunk domain.LawChunk) error {
	dbChunk := KnowledgeChunk{
		ProjectID:    projectID,
		DocName:      getStringFromMetadata(chunk.Metadata, "law_name", "未知文档"),
		ChunkContent: chunk.Content,
		ChunkIndex:   getIntFromMetadata(chunk.Metadata, "chunk_index", 0),
		Embedding:    pgvector.NewVector(chunk.Vector),
		Metadata:     chunk.Metadata,
	}

	// 设置 QA 内容和向量
	if chunk.QAContent != "" && len(chunk.QAVector) > 0 {
		dbChunk.QAContent = chunk.QAContent
		v := pgvector.NewVector(chunk.QAVector)
		dbChunk.QAEmbedding = &v
	}

	if err := r.db.WithContext(ctx).Create(&dbChunk).Error; err != nil {
		return fmt.Errorf("failed to insert chunk: %w", err)
	}

	return nil
}

func (r *PgVectorRepository) BatchInsertChunks(ctx context.Context, projectID int64, chunks []domain.LawChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// 转换为数据库模型
	dbChunks := make([]KnowledgeChunk, len(chunks))
	for i, chunk := range chunks {
		dbChunk := KnowledgeChunk{
			ProjectID:    projectID,
			DocName:      getStringFromMetadata(chunk.Metadata, "law_name", "未知文档"),
			ChunkContent: chunk.Content,
			ChunkIndex:   getIntFromMetadata(chunk.Metadata, "chunk_index", i),
			Embedding:    pgvector.NewVector(chunk.Vector),
			Metadata:     chunk.Metadata,
		}
		// 设置 QA 内容和向量
		if chunk.QAContent != "" && len(chunk.QAVector) > 0 {
			dbChunk.QAContent = chunk.QAContent
			v := pgvector.NewVector(chunk.QAVector)
			dbChunk.QAEmbedding = &v
		}
		dbChunks[i] = dbChunk
	}

	// 动态计算批量大小：根据每条记录的平均大小调整
	// 向量维度是 1024，每个 float32 占 4 字节，约 4KB
	// 加上其他字段，每条约 5-6KB
	// 批次大小建议：50-200 之间动态调整
	avgChunkSize := 0
	for _, chunk := range dbChunks {
		avgChunkSize += len(chunk.ChunkContent)
	}
	avgChunkSize /= len(dbChunks)

	// 根据内容大小动态调整批次大小
	var batchSize int
	if avgChunkSize < 500 {
		batchSize = 200 // 小文本，大批次
	} else if avgChunkSize < 2000 {
		batchSize = 100 // 中等文本，中批次
	} else {
		batchSize = 50 // 大文本，小批次
	}

	// 使用事务确保数据一致性
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r) // 重新抛出 panic
		}
	}()

	// 分批插入
	maxRetries := 3
	for i := 0; i < len(dbChunks); i += batchSize {
		end := i + batchSize
		if end > len(dbChunks) {
			end = len(dbChunks)
		}

		batch := dbChunks[i:end]
		var err error
		for retry := 0; retry < maxRetries; retry++ {
			err = tx.Create(batch).Error
			if err == nil {
				break // 成功，跳出重试循环
			}
			// 如果是唯一约束冲突或其他错误，不再重试
			if retry == maxRetries-1 {
				tx.Rollback()
				return fmt.Errorf("failed to batch insert chunks (batch %d-%d): %w", i, end, err)
			}
			logger.Log.Warn("Batch insert failed, retrying",
				zap.Int("batch_start", i),
				zap.Int("batch_end", end),
				zap.Int("retry", retry+1),
				zap.Error(err))
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *PgVectorRepository) GetLawDocuments(ctx context.Context, projectIDs []int64) ([]domain.LawDocument, error) {
	type DocStat struct {
		DocName      string
		ArticleCount int
	}

	var stats []DocStat
	query := r.db.WithContext(ctx).
		Model(&KnowledgeChunk{}).
		Select("doc_name, COUNT(*) as article_count").
		Group("doc_name").
		Order("doc_name ASC")

	if len(projectIDs) > 0 {
		query = query.Where("project_id IN ?", projectIDs)
	}

	err := query.Scan(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get law documents: %w", err)
	}

	result := make([]domain.LawDocument, 0, len(stats))
	for _, stat := range stats {
		result = append(result, domain.LawDocument{
			DocName:      stat.DocName,
			ArticleCount: stat.ArticleCount,
		})
	}

	return result, nil
}

func (r *PgVectorRepository) GetLawArticles(ctx context.Context, projectID int64, docName string) ([]domain.LawChunk, error) {
	var chunks []KnowledgeChunk

	err := r.db.WithContext(ctx).
		Where("project_id = ? AND doc_name = ?", projectID, docName).
		Order("chunk_index ASC").
		Find(&chunks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get law articles: %w", err)
	}

	return r.convertToDomainChunks(chunks), nil
}

func (r *PgVectorRepository) convertToDomainChunks(chunks []KnowledgeChunk) []domain.LawChunk {
	result := make([]domain.LawChunk, 0, len(chunks))

	for _, chunk := range chunks {
		metadata := chunk.Metadata
		if metadata == nil {
			metadata = make(map[string]interface{})
		}

		if _, ok := metadata["law_name"]; !ok {
			metadata["law_name"] = chunk.DocName
		}
		if _, ok := metadata["chunk_index"]; !ok {
			metadata["chunk_index"] = chunk.ChunkIndex
		}

		qaContent := ""
		var qaVector []float32
		if chunk.QAEmbedding != nil {
			qaContent = chunk.QAContent
			qaVector = chunk.QAEmbedding.Slice()
		}

		result = append(result, domain.LawChunk{
			ID:        chunk.ID,
			Content:   chunk.ChunkContent,
			QAContent: qaContent,
			Vector:    chunk.Embedding.Slice(),
			QAVector:  qaVector,
			Metadata:  metadata,
			CreatedAt: chunk.CreatedAt,
			UpdatedAt: chunk.UpdatedAt,
		})
	}

	return result
}

func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
	if val, ok := metadata[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getIntFromMetadata(metadata map[string]interface{}, key string, defaultValue int) int {
	if val, ok := metadata[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

func joinInt64(ids []int64, sep string) string {
	if len(ids) == 0 {
		return ""
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(strs, sep)
}

// GetChunkByID 根据 ID 获取 chunk
func (r *PgVectorRepository) GetChunkByID(ctx context.Context, id int64) (*domain.LawChunk, error) {
	var chunk KnowledgeChunk
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&chunk).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get chunk by id: %w", err)
	}
	if chunk.ID == 0 {
		return nil, nil
	}
	domainChunks := r.convertToDomainChunks([]KnowledgeChunk{chunk})
	if len(domainChunks) == 0 {
		return nil, nil
	}
	return &domainChunks[0], nil
}

// UpdateChunk 更新 chunk
func (r *PgVectorRepository) UpdateChunk(ctx context.Context, projectID int64, chunk domain.LawChunk) error {
	dbChunk := KnowledgeChunk{
		ID:           chunk.ID,
		ProjectID:    projectID,
		DocName:      getStringFromMetadata(chunk.Metadata, "law_name", "未知文档"),
		ChunkContent: chunk.Content,
		ChunkIndex:   getIntFromMetadata(chunk.Metadata, "chunk_index", 0),
		Embedding:    pgvector.NewVector(chunk.Vector),
		Metadata:     chunk.Metadata,
	}

	// 设置 QA 内容和向量
	if chunk.QAContent != "" && len(chunk.QAVector) > 0 {
		dbChunk.QAContent = chunk.QAContent
		v := pgvector.NewVector(chunk.QAVector)
		dbChunk.QAEmbedding = &v
	}

	result := r.db.WithContext(ctx).Model(&KnowledgeChunk{}).
		Where("id = ? AND project_id = ?", chunk.ID, projectID).
		Updates(map[string]interface{}{
			"chunk_content": dbChunk.ChunkContent,
			"embedding":     dbChunk.Embedding,
			"qa_content":    dbChunk.QAContent,
			"qa_embedding":  dbChunk.QAEmbedding,
			"metadata":      dbChunk.Metadata,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update chunk: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("chunk not found")
	}

	return nil
}

// DeleteChunk 删除 chunk
func (r *PgVectorRepository) DeleteChunk(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&KnowledgeChunk{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete chunk: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("chunk not found")
	}
	return nil
}
