package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"law-enforcement-brain/pkg/logger"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestIDHeader 请求 ID header 键
const RequestIDHeader = "X-Request-ID"

// RequestLogger 请求日志中间件
// 自动注入 request_id 并记录请求日志
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取或生成 request_id
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = generateRequestID()
		}

		// 注入 context
		ctx := logger.ContextWithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)

		// 设置响应 header
		c.Header(RequestIDHeader, requestID)

		// 记录请求开始
		start := time.Now()
		path := c.Request.URL.Path

		logger.Ctx(ctx).Info("Request started",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("client_ip", c.ClientIP()),
		)

		// 处理请求
		c.Next()

		// 记录请求结束
		latency := time.Since(start)
		logger.Ctx(ctx).Info("Request completed",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
		)
	}
}

// generateRequestID 生成请求 ID
func generateRequestID() string {
	timestamp := time.Now().Format("20060102150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	return timestamp + "-" + hex.EncodeToString(randomBytes)
}
