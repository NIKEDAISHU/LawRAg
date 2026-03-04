package main

import (
	"law-enforcement-brain/internal/adapter/llm"
	"law-enforcement-brain/internal/adapter/repository"
	"law-enforcement-brain/internal/api/handler"
	"law-enforcement-brain/internal/api/middleware"
	"law-enforcement-brain/internal/core/port"
	"law-enforcement-brain/internal/core/service"
	"law-enforcement-brain/pkg/config"
	"law-enforcement-brain/pkg/logger"
	"log"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	if err := logger.Init(); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	cfg := config.Load()

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logger.Log.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Log.Info("Starting law-enforcement-brain service",
		zap.String("port", cfg.Server.Port),
		zap.String("db_host", cfg.Database.Host),
		zap.String("llm_base_url", cfg.LLM.BaseURL))

	lawRepo, db, err := repository.NewPgVectorRepository(&cfg.Database)
	if err != nil {
		logger.Log.Fatal("Failed to initialize repository", zap.Error(err))
	}
	logger.Log.Info("Database connection established")

	projectRepo := repository.NewProjectRepository(db)
	logger.Log.Info("Project repository initialized")

	var llmClient port.LLMClient
	// 根据 Provider 选择不同的 LLM 客户端
	switch cfg.LLM.Provider {
	case "minimax":
		llmClient = llm.NewMiniMaxClient(&cfg.LLM)
		logger.Log.Info("MiniMax LLM client initialized",
			zap.String("base_url", "https://api.minimaxi.com/anthropic"),
			zap.String("model", cfg.LLM.Model))
	case "qwen":
		llmClient = llm.NewQwenClient(&cfg.LLM)
		logger.Log.Info("Qwen LLM client initialized",
			zap.String("base_url", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
			zap.String("model", cfg.LLM.QwenModel))
	case "hybrid":
		llmClient = llm.NewHybridClient(&cfg.LLM)
		logger.Log.Info("Hybrid LLM client initialized",
			zap.String("chat_url", cfg.LLM.BaseURL),
			zap.String("chat_model", cfg.LLM.Model),
			zap.String("embedding_url", cfg.LLM.OllamaBaseURL),
			zap.String("embedding_model", cfg.LLM.EmbeddingModel))
	default:
		llmClient = llm.NewOpenAIClient(&cfg.LLM)
		logger.Log.Info("OpenAI LLM client initialized",
			zap.String("base_url", cfg.LLM.BaseURL))
	}

	projectService := service.NewProjectService(projectRepo)
	reviewService := service.NewReviewService(lawRepo, llmClient, &cfg.Review)
	logger.Log.Info("Services initialized")

	projectHandler := handler.NewProjectHandler(projectService)
	reviewHandler := handler.NewReviewHandler(reviewService, lawRepo, llmClient)
	uploadHandler := handler.NewUploadHandler(lawRepo, llmClient)
	knowledgeHandler := handler.NewKnowledgeHandler(lawRepo)
	chatHandler := handler.NewChatHandler(lawRepo, llmClient)
	logger.Log.Info("Handlers initialized")

	router := gin.Default()
	router.Use(middleware.RequestLogger())

	// 启用 CORS
	// 如果配置了 CORS_ALLOWED_ORIGINS，使用配置的来源；否则允许所有（开发环境）
	var allowedOrigins []string
	if cfg.Server.CORSAllowedOrigins != "" {
		allowedOrigins = strings.Split(cfg.Server.CORSAllowedOrigins, ",")
		for i := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		}
		logger.Log.Info("CORS configured with specific origins", zap.Strings("origins", allowedOrigins))
	} else {
		allowedOrigins = []string{"*"}
		logger.Log.Warn("CORS is allowing all origins (*). This is insecure for production!")
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 静态文件服务
	router.Static("/web", "./web")
	router.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/web/index.html")
	})

	// API 路由
	router.GET("/health", reviewHandler.HealthCheck)
	router.POST("/api/v1/review", reviewHandler.ReviewCase)
	router.POST("/api/v1/upload", uploadHandler.UploadLawDocument)
	router.POST("/api/v1/import-url", uploadHandler.ImportFromURL)
	router.GET("/api/v1/stats", uploadHandler.ListLawDocuments)
	router.POST("/api/v1/search", uploadHandler.SearchLawDocuments)

	// 项目管理接口
	router.POST("/api/v1/projects", projectHandler.CreateProject)
	router.GET("/api/v1/projects", projectHandler.ListProjects)
	router.GET("/api/v1/projects/:id", projectHandler.GetProject)
	router.PUT("/api/v1/projects/:id", projectHandler.UpdateProject)
	router.DELETE("/api/v1/projects/:id", projectHandler.DeleteProject)
	router.GET("/api/v1/projects/:id/statistics", projectHandler.GetProjectStatistics)
	router.GET("/api/v1/projects/statistics", projectHandler.ListProjectStatistics)

	// 知识库接口
	router.GET("/api/v1/knowledge/documents", knowledgeHandler.GetLawDocuments)
	router.GET("/api/v1/knowledge/documents/:doc_name/articles", knowledgeHandler.GetLawArticles)

	// LLM 对话接口
	router.POST("/api/v1/chat", chatHandler.Chat)

	// Dify 兼容接口（Java 端调用）
	router.POST("/chat-messages", reviewHandler.DifyChatCompletion)
	router.POST("/v1/chat-messages", reviewHandler.DifyChatCompletion) // 兼容两种路径

	addr := "0.0.0.0:8888"
	logger.Log.Info("Server starting", zap.String("address", addr))

	if err := router.Run(addr); err != nil {
		logger.Log.Fatal("Failed to start server", zap.Error(err))
	}
}
