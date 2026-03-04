package domain

import "time"

type ReviewRequest struct {
	RequestID string   `json:"request_id" binding:"required"`
	CaseInfo  CaseInfo `json:"case_info" binding:"required"`
	Config    Config   `json:"config"`
}

type CaseInfo struct {
	CaseType string     `json:"case_type" binding:"required"`
	DocList  []Document `json:"doc_list" binding:"required,min=1"`
}

type Document struct {
	Filename string `json:"filename" binding:"required"`
	Content  string `json:"content" binding:"required"`
	Type     string `json:"type" binding:"required"`
}

type Config struct {
	CheckStrictness string `json:"check_strictness"`
}

type ReviewResponse struct {
	Code int          `json:"code"`
	Data ReviewResult `json:"data,omitempty"`
	Msg  string       `json:"msg,omitempty"`
}

type ReviewResult struct {
	OverallResult string  `json:"overall_result"`
	RiskScore     int     `json:"risk_score"`
	Issues        []Issue `json:"issues"`
}

type Issue struct {
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Severity      string        `json:"severity"`
	RelatedDoc    string        `json:"related_doc"`
	ReferenceLaw  string        `json:"reference_law"`
	Citations     []LawCitation `json:"citations"` // 结构化法律引用
}

// LawCitation 结构化法律引用
type LawCitation struct {
	LawID    string  `json:"law_id"`    // 法规 ID（如 "治安管理处罚法"）
	Article  string  `json:"article"`   // 条款（如 "第25条"）
	Content  string  `json:"content"`  // 引用的具体内容
	Confidence float64 `json:"confidence"` // 置信度 0-1
	Verified  bool    `json:"verified"`  // 是否已通过二次校验
}

type LawChunk struct {
	ID        int64
	Content   string
	Vector    []float32
	QAContent string    // QA 对内容
	QAVector  []float32 // QA 向量
	Metadata  map[string]interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ExtractedInfo struct {
	ViolationFacts string   `json:"violation_facts"`
	CitedLaws      []string `json:"cited_laws"`
	KeyFacts       []string `json:"key_facts"`
}

type ValidationResult struct {
	IsCorrect bool    `json:"is_correct"`
	Issues    []Issue `json:"issues"`
	RiskScore int     `json:"risk_score"`
}

type LawDocument struct {
	DocName      string `json:"doc_name"`
	ArticleCount int    `json:"article_count"`
}

type Project struct {
	ID             int64                  `json:"id"`
	ProjectCode    string                 `json:"project_code"`
	ProjectName    string                 `json:"project_name"`
	ProjectType    string                 `json:"project_type"`
	Description    string                 `json:"description"`
	EmbeddingModel string                 `json:"embedding_model"`
	VectorDim      int                    `json:"vector_dim"`
	Config         map[string]interface{} `json:"config"`
	IsActive       bool                   `json:"is_active"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type ProjectStatistics struct {
	Project
	DocCount   int `json:"doc_count"`
	ChunkCount int `json:"chunk_count"`
}

type CreateProjectRequest struct {
	ProjectCode    string                 `json:"project_code" binding:"required"`
	ProjectName    string                 `json:"project_name" binding:"required"`
	ProjectType    string                 `json:"project_type"`
	Description    string                 `json:"description"`
	EmbeddingModel string                 `json:"embedding_model"`
	VectorDim      int                    `json:"vector_dim"`
	Config         map[string]interface{} `json:"config"`
}

type UpdateProjectRequest struct {
	ProjectName string                 `json:"project_name"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	IsActive    *bool                  `json:"is_active"`
}
