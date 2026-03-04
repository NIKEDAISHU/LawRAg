package handler

import (
	"fmt"
	"io"
	"law-enforcement-brain/internal/core/domain"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/pkg/logger"
	"law-enforcement-brain/pkg/qa"
	"law-enforcement-brain/pkg/splitter"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

type UploadHandler struct {
	lawRepo            port.LawRepository
	llmClient          port.LLMClient
	lawSplitter        *splitter.LawSplitter
	mdSplitter         *splitter.MdSplitter
	discretionSplitter *splitter.DiscretionSplitter
}

func NewUploadHandler(lawRepo port.LawRepository, llmClient port.LLMClient) *UploadHandler {
	return &UploadHandler{
		lawRepo:            lawRepo,
		llmClient:          llmClient,
		lawSplitter:        splitter.NewLawSplitter(),
		mdSplitter:         splitter.NewMdSplitter(),
		discretionSplitter: splitter.NewDiscretionSplitter(),
	}
}

type UploadResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ChunksCount  int    `json:"chunks_count,omitempty"`
	DocumentName string `json:"document_name,omitempty"`
}

// UploadLawDocument 上传法律文档并生成向量
func (h *UploadHandler) UploadLawDocument(c *gin.Context) {
	// 获取项目ID参数
	projectIDStr := c.PostForm("project_id")
	if projectIDStr == "" {
		projectIDStr = c.DefaultPostForm("project_id", "1")
	}
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		logger.Log.Error("Invalid project_id", zap.String("project_id", projectIDStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, UploadResponse{
			Success: false,
			Message: "无效的项目ID",
		})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		logger.Log.Error("Failed to get file", zap.Error(err))
		c.JSON(http.StatusBadRequest, UploadResponse{
			Success: false,
			Message: "未找到上传文件",
		})
		return
	}

	// 检查文件类型
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".txt" && ext != ".md" {
		c.JSON(http.StatusBadRequest, UploadResponse{
			Success: false,
			Message: "仅支持 .txt 和 .md 文件",
		})
		return
	}

	// 读取文件内容
	f, err := file.Open()
	if err != nil {
		logger.Log.Error("Failed to open file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, UploadResponse{
			Success: false,
			Message: "文件读取失败",
		})
		return
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		logger.Log.Error("Failed to read file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, UploadResponse{
			Success: false,
			Message: "文件内容读取失败",
		})
		return
	}

	// 根据文件内容自动识别切分器
	var chunks []domain.LawChunk
	contentStr := string(content)

	// 检测是否为裁量基准格式（包含【违法行为】和 ======）
	if strings.Contains(contentStr, "【违法行为】") && strings.Contains(contentStr, "======") {
		chunks = h.discretionSplitter.Split(contentStr)
		logger.Log.Info("Discretion standard document split into chunks",
			zap.String("filename", file.Filename),
			zap.Int("chunks", len(chunks)))
	} else if ext == ".md" || strings.Contains(contentStr, "---") {
		// MD 格式或包含 --- 分隔符
		chunks = h.mdSplitter.Split(contentStr)
		logger.Log.Info("MD document split into chunks",
			zap.String("filename", file.Filename),
			zap.Int("chunks", len(chunks)))
	} else {
		// 默认使用法律条文切分器
		chunks = h.lawSplitter.Split(contentStr)
		logger.Log.Info("Law document split into chunks",
			zap.String("filename", file.Filename),
			zap.Int("chunks", len(chunks)))
	}

	// 生成向量并存储
	ctx := c.Request.Context()
	successCount := 0

	// 获取 generate_qa 参数，默认不生成 QA
	generateQA := c.PostForm("generate_qa") == "true"

	if generateQA {
		logger.Log.Info("QA generation enabled for upload",
			zap.String("filename", file.Filename))
	}

	// 使用并发处理 chunks 以提高性能
	const maxWorkers = 10 // 限制并发数，避免过载
	type ProcessResult struct {
		Index     int
		Chunk     domain.LawChunk
		Embedding []float32
		QAContent string
		QAVector  []float32
		Metadata  map[string]string
		Error     error
	}

	resultChan := make(chan ProcessResult, len(chunks))
	semaphore := make(chan struct{}, maxWorkers)

	// 启动 worker goroutines 处理每个 chunk
	for i, chunk := range chunks {
		go func(index int, ch domain.LawChunk) {
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			result := ProcessResult{Index: index, Chunk: ch}

			// 生成嵌入向量
			embedding, err := h.llmClient.CreateEmbedding(ctx, ch.Content)
			if err != nil {
				result.Error = fmt.Errorf("failed to create embedding: %w", err)
				resultChan <- result
				return
			}
			result.Embedding = embedding

			// 只有当用户选择生成 QA 时才调用 LLM
			if generateQA {
				qaGenerator := qa.NewGenerator(h.llmClient)

				// 生成 QA 内容和向量
				docName := ch.Content
				if name, ok := ch.Metadata["law_name"]; ok {
					if s, ok := name.(string); ok {
						docName = s
					}
				}
				qaReq := qa.GenerateQA{
					ChunkID:   int64(index),
					Content:   ch.Content,
					DocName:   docName,
					Knowledge: docName,
				}
				qaResult, err := qaGenerator.GenerateForChunks(ctx, []qa.GenerateQA{qaReq})
				if err == nil && len(qaResult) > 0 {
					result.QAContent = qaResult[int64(index)]
					// 生成 QA 向量
					if result.QAContent != "" {
						qaVector, err := h.llmClient.CreateEmbedding(ctx, result.QAContent)
						if err != nil {
							logger.Log.Warn("Failed to create QA embedding",
								zap.Int("chunk_index", index),
								zap.Error(err))
						} else {
							result.QAVector = qaVector
						}
					}
				} else if err != nil {
					logger.Log.Warn("Failed to generate QA",
						zap.Int("chunk_index", index),
						zap.Error(err))
				}
			}

			// 转换 metadata 类型并添加文件名
			metadata := make(map[string]string)
			metadata["filename"] = file.Filename
			metadata["chunk_index"] = fmt.Sprintf("%d", index)

			// 从 chunk.Metadata 中提取所有字段
			for k, v := range ch.Metadata {
				switch val := v.(type) {
				case string:
					metadata[k] = val
				case int:
					metadata[k] = fmt.Sprintf("%d", val)
				default:
					metadata[k] = fmt.Sprintf("%v", val)
				}
			}
			result.Metadata = metadata

			resultChan <- result
		}(i, chunk)
	}

	// 收集所有结果
	results := make([]ProcessResult, 0, len(chunks))
	for i := 0; i < len(chunks); i++ {
		result := <-resultChan
		if result.Error != nil {
			logger.Log.Error("Failed to process chunk",
				zap.Int("chunk_index", result.Index),
				zap.Error(result.Error))
			continue
		}
		results = append(results, result)
	}
	close(resultChan)

	// 按原始顺序排序并存储到数据库
	for _, result := range results {
		err := h.lawRepo.StoreWithQA(ctx, projectID, result.Chunk.Content, result.Embedding, result.QAContent, result.QAVector, result.Metadata)
		if err != nil {
			logger.Log.Error("Failed to store chunk",
				zap.Int("chunk_index", result.Index),
				zap.Error(err))
			continue
		}
		successCount++
	}

	logger.Log.Info("Law document processed",
		zap.String("filename", file.Filename),
		zap.Int("total_chunks", len(chunks)),
		zap.Int("success_count", successCount))

	c.JSON(http.StatusOK, UploadResponse{
		Success:      true,
		Message:      fmt.Sprintf("成功导入 %d/%d 个法律条文", successCount, len(chunks)),
		ChunksCount:  successCount,
		DocumentName: file.Filename,
	})
}

// ListLawDocuments 列出已导入的法律文档统计
func (h *UploadHandler) ListLawDocuments(c *gin.Context) {
	// 这里简化处理，实际可以从数据库查询统计信息
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "查询成功",
		"data": gin.H{
			"total_chunks": "请查询数据库获取准确数量",
		},
	})
}

// SearchLawDocuments 搜索法律文档
func (h *UploadHandler) SearchLawDocuments(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "查询内容不能为空",
		})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 5
	}

	ctx := c.Request.Context()

	// 生成查询向量
	queryVector, err := h.llmClient.CreateEmbedding(ctx, req.Query)
	if err != nil {
		logger.Log.Error("Failed to create query embedding", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "查询向量生成失败",
		})
		return
	}

	// 转换为 pgvector 格式
	vectorStr := fmt.Sprintf("[%v]", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(queryVector)), ","), "[]"))

	// 搜索相似文档（召回更多候选，后续重排序）
	recallTopK := req.TopK * 3 // 召回 3 倍数量
	if recallTopK > 20 {
		recallTopK = 20
	}

	// 默认搜索项目ID=1
	defaultProjectIDs := []int64{domain.DefaultProjectID}
	chunks, err := h.lawRepo.SearchSimilar(ctx, defaultProjectIDs, vectorStr, recallTopK)
	if err != nil {
		logger.Log.Error("Failed to search similar", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "搜索失败",
		})
		return
	}

	// 简单的关键词重排序
	rankedChunks := rerankByKeywords(chunks, req.Query)

	// 截取 top_k
	if len(rankedChunks) > req.TopK {
		rankedChunks = rankedChunks[:req.TopK]
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "搜索成功",
		"data": gin.H{
			"query":   req.Query,
			"results": rankedChunks,
			"count":   len(rankedChunks),
		},
	})
}

// DeleteAllLawDocuments 清空所有法律文档（谨慎使用）
func (h *UploadHandler) DeleteAllLawDocuments(c *gin.Context) {
	// 需要确认密码
	password := c.PostForm("password")
	if password != "delete_all_confirm" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "需要确认密码",
		})
		return
	}

	// 这里需要在 repository 中实现 DeleteAll 方法
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "清空功能需要在 repository 中实现",
	})
}

// ImportFromURL 从 URL 导入法律文档
func (h *UploadHandler) ImportFromURL(c *gin.Context) {
	var req struct {
		URL        string `json:"url" binding:"required"`
		ProjectID  int64  `json:"project_id"`
		GenerateQA bool   `json:"generate_qa"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, UploadResponse{
			Success: false,
			Message: "URL 不能为空",
		})
		return
	}

	// 如果没有提供project_id，使用默认值
	projectID := req.ProjectID
	if projectID == 0 {
		projectID = domain.DefaultProjectID
	}

	// 验证 URL 格式
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		c.JSON(http.StatusBadRequest, UploadResponse{
			Success: false,
			Message: "URL 格式不正确",
		})
		return
	}

	// 抓取网页内容
	resp, err := http.Get(req.URL)
	if err != nil {
		logger.Log.Error("Failed to fetch URL", zap.Error(err))
		c.JSON(http.StatusInternalServerError, UploadResponse{
			Success: false,
			Message: "无法访问该 URL",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, UploadResponse{
			Success: false,
			Message: fmt.Sprintf("URL 返回错误: %d", resp.StatusCode),
		})
		return
	}

	// 解析 HTML 提取文本
	doc, err := html.Parse(resp.Body)
	if err != nil {
		logger.Log.Error("Failed to parse HTML", zap.Error(err))
		c.JSON(http.StatusInternalServerError, UploadResponse{
			Success: false,
			Message: "HTML 解析失败",
		})
		return
	}

	// 提取文本内容
	content := extractText(doc)

	// 清理文本
	content = cleanText(content)

	if len(content) < 50 {
		c.JSON(http.StatusBadRequest, UploadResponse{
			Success: false,
			Message: "提取的内容过少，请检查 URL",
		})
		return
	}

	// 从 URL 提取文档名
	docName := extractDocNameFromURL(req.URL)

	// 切分法律条文
	chunks := h.lawSplitter.Split(content)
	logger.Log.Info("URL content split into chunks",
		zap.String("url", req.URL),
		zap.Int("chunks", len(chunks)))

	// 生成向量并存储
	ctx := c.Request.Context()
	successCount := 0

	// 获取 generate_qa 参数，默认不生成 QA
	generateQA := req.GenerateQA

	if generateQA {
		logger.Log.Info("QA generation enabled for URL import",
			zap.String("url", req.URL))
	}

	for i, chunk := range chunks {
		// 生成嵌入向量
		embedding, err := h.llmClient.CreateEmbedding(ctx, chunk.Content)
		if err != nil {
			logger.Log.Error("Failed to create embedding",
				zap.Int("chunk_index", i),
				zap.Error(err))
			continue
		}

		var qaContent string
		var qaVector []float32

		// 只有当用户选择生成 QA 时才调用 LLM
		if generateQA {
			qaGenerator := qa.NewGenerator(h.llmClient)

			// 生成 QA 内容和向量
			qaReq := qa.GenerateQA{
				ChunkID:   int64(i),
				Content:   chunk.Content,
				DocName:   docName,
				Knowledge: docName,
			}
			qaResult, err := qaGenerator.GenerateForChunks(ctx, []qa.GenerateQA{qaReq})
			if err == nil && len(qaResult) > 0 {
				qaContent = qaResult[int64(i)]
				if qaContent != "" {
					qaVector, err = h.llmClient.CreateEmbedding(ctx, qaContent)
					if err != nil {
						logger.Log.Warn("Failed to create QA embedding",
							zap.Int("chunk_index", i),
							zap.Error(err))
						qaVector = nil
					}
				}
			} else if err != nil {
				logger.Log.Warn("Failed to generate QA",
					zap.Int("chunk_index", i),
					zap.Error(err))
			}
		}

		// 构建 metadata
		metadata := make(map[string]string)
		metadata["filename"] = docName
		metadata["source_url"] = req.URL
		metadata["chunk_index"] = fmt.Sprintf("%d", i)
		for k, v := range chunk.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			}
		}

		// 存储到数据库（包含 QA 内容和向量）
		err = h.lawRepo.StoreWithQA(ctx, projectID, chunk.Content, embedding, qaContent, qaVector, metadata)
		if err != nil {
			logger.Log.Error("Failed to store chunk",
				zap.Int("chunk_index", i),
				zap.Error(err))
			continue
		}

		successCount++
	}

	logger.Log.Info("URL content processed",
		zap.String("url", req.URL),
		zap.Int("total_chunks", len(chunks)),
		zap.Int("success_count", successCount))

	c.JSON(http.StatusOK, UploadResponse{
		Success:      true,
		Message:      fmt.Sprintf("成功从 URL 导入 %d/%d 个法律条文", successCount, len(chunks)),
		ChunksCount:  successCount,
		DocumentName: docName,
	})
}

// extractTitle 从 HTML 中提取标题
func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
			title := strings.TrimSpace(n.FirstChild.Data)
			// 清理标题中的特殊字符
			title = strings.ReplaceAll(title, "_", "")
			title = strings.ReplaceAll(title, "-", "")
			return title
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := extractTitle(c); title != "" {
			return title
		}
	}

	return ""
}

// extractText 从 HTML 节点中提取文本
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}

	// 跳过 script 和 style 标签
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
		return ""
	}

	var text string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += extractText(c)
	}

	// 在块级元素后添加换行
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li":
			text += "\n"
		}
	}

	return text
}

// cleanText 清理提取的文本
func cleanText(text string) string {
	// 移除多余的空白字符
	re := regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	// 移除行首行尾空白
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
}

// extractDocNameFromURL 从 URL 中提取文档名
func extractDocNameFromURL(url string) string {
	// 尝试从 URL 路径中提取有意义的名称
	parts := strings.Split(url, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" && !strings.Contains(parts[i], ".") {
			return parts[i]
		}
	}

	// 如果无法提取，使用域名
	re := regexp.MustCompile(`https?://([^/]+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}

	return "网页文档"
}

// rerankByKeywords 基于关键词匹配对结果重排序
func rerankByKeywords(chunks []domain.LawChunk, query string) []domain.LawChunk {
	// 提取查询中的关键词（简单分词）
	keywords := extractKeywords(query)

	// 计算每个 chunk 的得分
	type scoredChunk struct {
		chunk domain.LawChunk
		score int
	}

	scored := make([]scoredChunk, 0, len(chunks))
	for _, chunk := range chunks {
		score := 0
		content := strings.ToLower(chunk.Content)

		// 计算关键词匹配数
		for _, keyword := range keywords {
			if strings.Contains(content, keyword) {
				score += strings.Count(content, keyword) * 10
			}
		}

		// 完整查询匹配加分
		if strings.Contains(content, strings.ToLower(query)) {
			score += 50
		}

		scored = append(scored, scoredChunk{chunk: chunk, score: score})
	}

	// 按得分排序（降序）- 使用 sort.Slice 替代冒泡排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 提取排序后的 chunks
	result := make([]domain.LawChunk, 0, len(scored))
	for _, s := range scored {
		result = append(result, s.chunk)
	}

	return result
}

// extractKeywords 提取查询中的关键词
func extractKeywords(query string) []string {
	query = strings.ToLower(query)

	// 移除常见停用词
	stopWords := map[string]bool{
		"的": true, "是": true, "在": true, "有": true, "和": true,
		"与": true, "及": true, "或": true, "等": true, "了": true,
		"吗": true, "呢": true, "啊": true, "什么": true, "如何": true,
		"怎么": true, "怎样": true, "为什么": true,
	}

	// 简单分词（按空格和标点）
	re := regexp.MustCompile(`[\s，。！？、；：""''（）《》【】]+`)
	words := re.Split(query, -1)

	keywords := make([]string, 0)
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}
