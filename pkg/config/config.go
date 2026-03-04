package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// 配置验证错误
var (
	ErrEmptyDBPassword = errors.New("database password is required")
	ErrEmptyLLMAPIKey  = errors.New("LLM API key is required")
	ErrEmptyDBHost     = errors.New("database host is required")
	ErrEmptyDBUser     = errors.New("database user is required")
	ErrEmptyDBName     = errors.New("database name is required")
	ErrInvalidPort     = errors.New("invalid port number")
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	LLM      LLMConfig
	Review   ReviewConfig
}

type ServerConfig struct {
	Port               string
	CORSAllowedOrigins string // 允许的 CORS 来源，多个用逗号分隔，空字符串表示允许所有
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

type LLMConfig struct {
	Provider        string // 可选: openai, minimax, hybrid, qwen
	BaseURL         string
	APIKey          string
	Model           string
	EmbeddingModel  string
	OllamaBaseURL   string
	VectorDim       int
	LocalOCRBaseURL string // 本地 OCR 模型地址（用于 RewriteQuery）
	LocalOCRModel   string // 本地 OCR 模型名称
	// 阿里百炼 Qwen 配置
	QwenAPIKey string // 阿里百炼 API Key
	QwenModel  string // 千问模型名称，如 qwen2.5-plus
}

// LLM Provider 常量
const (
	ProviderOpenAI  = "openai"
	ProviderMiniMax = "minimax"
	ProviderQwen    = "qwen"
	ProviderHybrid  = "hybrid"
)

// GetActiveLLMConfig 获取当前 Provider 对应的有效配置
func (c *LLMConfig) GetActiveLLMConfig() (baseURL, apiKey, model string) {
	switch c.Provider {
	case ProviderMiniMax:
		return "https://api.minimaxi.com/anthropic", c.APIKey, c.Model
	case ProviderQwen:
		return "https://dashscope.aliyuncs.com/compatible-mode/v1", c.QwenAPIKey, c.QwenModel
	case ProviderHybrid:
		return c.BaseURL, c.APIKey, c.Model
	default:
		return c.BaseURL, c.APIKey, c.Model
	}
}

type ReviewConfig struct {
	CheckStrictness string
	FallbackMode    bool
}

// loadEnvFile 从指定目录加载 .env 文件
func loadEnvFile(dir string) error {
	envPath := filepath.Join(dir, ".env")
	return godotenv.Load(envPath)
}

func Load() *Config {
	// 加载 .env 文件（从项目根目录查找）
	// 尝试从当前工作目录和父目录查找 .env 文件
	_ = loadEnvFile(".")

	// 如果当前目录没找到，尝试从父目录查找
	if os.Getenv("DB_PASSWORD") == "" {
		_ = loadEnvFile("..")
	}

	// 再尝试从更上一级目录查找（当从 cmd/api 运行时）
	if os.Getenv("DB_PASSWORD") == "" {
		_ = loadEnvFile("../..")
	}

	return &Config{
		Server: ServerConfig{
			Port:               getEnv("SERVER_PORT", "8080"),
			CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", ""),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "law_enforcement"),
		},
		LLM: LLMConfig{
			Provider:        getEnv("LLM_PROVIDER", "openai"),
			BaseURL:         getEnv("LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
			APIKey:          getEnv("LLM_API_KEY", ""),
			Model:           getEnv("LLM_MODEL", "deepseek-v3.2"),
			EmbeddingModel:  getEnv("LLM_EMBEDDING_MODEL", "bge-m3"),
			OllamaBaseURL:   getEnv("OLLAMA_BASE_URL", "http://localhost:11434"),
			VectorDim:       getEnvInt("VECTOR_DIM", 1024),
			LocalOCRBaseURL: getEnv("LOCAL_OCR_BASE_URL", "http://localhost:11434"),
			LocalOCRModel:   getEnv("LOCAL_OCR_MODEL", "qwen2.5:7b"),
			// 阿里百炼 Qwen 配置
			QwenAPIKey: getEnv("QWEN_API_KEY", ""),
			QwenModel:  getEnv("QWEN_MODEL", "qwen2.5-plus"),
		},
		Review: ReviewConfig{
			CheckStrictness: getEnv("REVIEW_STRICTNESS", "high"),
			FallbackMode:    getEnvBool("REVIEW_FALLBACK_MODE", true),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// Validate 验证配置必填字段
func (c *Config) Validate() error {
	// 验证数据库配置
	if c.Database.Host == "" {
		return ErrEmptyDBHost
	}
	if c.Database.User == "" {
		return ErrEmptyDBUser
	}
	if c.Database.Password == "" {
		return ErrEmptyDBPassword
	}
	if c.Database.DBName == "" {
		return ErrEmptyDBName
	}

	// 验证 LLM 配置
	if c.LLM.APIKey == "" {
		return ErrEmptyLLMAPIKey
	}

	// 验证端口号
	if c.Server.Port != "" {
		port, err := strconv.Atoi(c.Server.Port)
		if err != nil || port < 1 || port > 65535 {
			return ErrInvalidPort
		}
	}

	return nil
}
