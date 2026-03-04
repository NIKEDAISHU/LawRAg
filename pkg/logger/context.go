package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	projectIDKey   contextKey = "project_id"
	llmProviderKey contextKey = "llm_provider"
	modelNameKey   contextKey = "model_name"
)

// ContextWithRequestID 将 request_id 注入 context
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// ContextWithProjectID 将 project_id 注入 context
func ContextWithProjectID(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, projectIDKey, projectID)
}

// ContextWithLLMProvider 将 llm_provider 注入 context
func ContextWithLLMProvider(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, llmProviderKey, provider)
}

// ContextWithModelName 将 model_name 注入 context
func ContextWithModelName(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, modelNameKey, model)
}

// GetRequestID 从 context 获取 request_id
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// GetProjectID 从 context 获取 project_id
func GetProjectID(ctx context.Context) string {
	if id, ok := ctx.Value(projectIDKey).(string); ok {
		return id
	}
	return ""
}

// GetLLMProvider 从 context 获取 llm_provider
func GetLLMProvider(ctx context.Context) string {
	if p, ok := ctx.Value(llmProviderKey).(string); ok {
		return p
	}
	return ""
}

// GetModelName 从 context 获取 model_name
func GetModelName(ctx context.Context) string {
	if m, ok := ctx.Value(modelNameKey).(string); ok {
		return m
	}
	return ""
}

// WithContextFields 将 context 中的字段注入 logger
func WithContextFields(ctx context.Context, log *zap.Logger) *zap.Logger {
	fields := []zapcore.Field{}

	if requestID := GetRequestID(ctx); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}
	if projectID := GetProjectID(ctx); projectID != "" {
		fields = append(fields, zap.String("project_id", projectID))
	}
	if provider := GetLLMProvider(ctx); provider != "" {
		fields = append(fields, zap.String("llm_provider", provider))
	}
	if model := GetModelName(ctx); model != "" {
		fields = append(fields, zap.String("model_name", model))
	}

	if len(fields) == 0 {
		return log
	}

	return log.With(fields...)
}

// Ctx 获取带有上下文信息的 logger
func Ctx(ctx context.Context) *zap.Logger {
	return WithContextFields(ctx, Log)
}
