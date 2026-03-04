package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// 保存原始环境变量
	origEnv := make(map[string]string)
	for _, key := range []string{"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "LLM_API_KEY", "SERVER_PORT"} {
		origEnv[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range origEnv {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	// 设置测试环境变量
	os.Setenv("DB_HOST", "testhost")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
	os.Setenv("LLM_API_KEY", "test-api-key")
	os.Setenv("SERVER_PORT", "8888")

	cfg := Load()

	if cfg.Database.Host != "testhost" {
		t.Errorf("expected DB_HOST to be 'testhost', got '%s'", cfg.Database.Host)
	}
	if cfg.Database.User != "testuser" {
		t.Errorf("expected DB_USER to be 'testuser', got '%s'", cfg.Database.User)
	}
	if cfg.Database.Password != "testpass" {
		t.Errorf("expected DB_PASSWORD to be 'testpass', got '%s'", cfg.Database.Password)
	}
	if cfg.Database.DBName != "testdb" {
		t.Errorf("expected DB_NAME to be 'testdb', got '%s'", cfg.Database.DBName)
	}
	if cfg.LLM.APIKey != "test-api-key" {
		t.Errorf("expected LLM_API_KEY to be 'test-api-key', got '%s'", cfg.LLM.APIKey)
	}
	if cfg.Server.Port != "8888" {
		t.Errorf("expected SERVER_PORT to be '8888', got '%s'", cfg.Server.Port)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{
			name: "valid config",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "postgres",
					Password: "password",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
				Server: ServerConfig{
					Port: "8080",
				},
			},
			wantErr: nil,
		},
		{
			name: "missing DB host",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "",
					User:     "postgres",
					Password: "password",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
			},
			wantErr: ErrEmptyDBHost,
		},
		{
			name: "missing DB user",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "",
					Password: "password",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
			},
			wantErr: ErrEmptyDBUser,
		},
		{
			name: "missing DB password",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "postgres",
					Password: "",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
			},
			wantErr: ErrEmptyDBPassword,
		},
		{
			name: "missing DB name",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "postgres",
					Password: "password",
					DBName:   "",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
			},
			wantErr: ErrEmptyDBName,
		},
		{
			name: "missing LLM API key",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "postgres",
					Password: "password",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "",
				},
			},
			wantErr: ErrEmptyLLMAPIKey,
		},
		{
			name: "invalid port",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "postgres",
					Password: "password",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
				Server: ServerConfig{
					Port: "invalid",
				},
			},
			wantErr: ErrInvalidPort,
		},
		{
			name: "port out of range",
			cfg: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					User:     "postgres",
					Password: "password",
					DBName:   "testdb",
				},
				LLM: LLMConfig{
					APIKey: "test-key",
				},
				Server: ServerConfig{
					Port: "70000",
				},
			},
			wantErr: ErrInvalidPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	// 保存原始值
	orig := os.Getenv("TEST_KEY")
	defer func() {
		if orig == "" {
			os.Unsetenv("TEST_KEY")
		} else {
			os.Setenv("TEST_KEY", orig)
		}
	}()

	// 测试环境变量存在
	os.Setenv("TEST_KEY", "test_value")
	result := getEnv("TEST_KEY", "default")
	if result != "test_value" {
		t.Errorf("expected 'test_value', got '%s'", result)
	}

	// 测试环境变量不存在
	os.Unsetenv("TEST_KEY")
	result = getEnv("TEST_KEY", "default")
	if result != "default" {
		t.Errorf("expected 'default', got '%s'", result)
	}
}

func TestGetEnvBool(t *testing.T) {
	// 保存原始值
	orig := os.Getenv("TEST_BOOL")
	defer func() {
		if orig == "" {
			os.Unsetenv("TEST_BOOL")
		} else {
			os.Setenv("TEST_BOOL", orig)
		}
	}()

	tests := []struct {
		name     string
		envValue string
		want     bool
		wantOrig bool
	}{
		{"true", "true", true, true},
		{"false", "false", false, false},
		{"1", "1", true, true},
		{"0", "0", false, false},
		{"invalid", "invalid", true, false},
		{"not set", "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("TEST_BOOL")
			} else {
				os.Setenv("TEST_BOOL", tt.envValue)
			}
			result := getEnvBool("TEST_BOOL", true)
			if result != tt.want {
				t.Errorf("expected %v, got %v", tt.want, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	// 保存原始值
	orig := os.Getenv("TEST_INT")
	defer func() {
		if orig == "" {
			os.Unsetenv("TEST_INT")
		} else {
			os.Setenv("TEST_INT", orig)
		}
	}()

	tests := []struct {
		name     string
		envValue string
		want     int
	}{
		{"valid", "123", 123},
		{"zero", "0", 0},
		{"negative", "-10", -10},
		{"invalid", "abc", 42},
		{"not set", "", 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("TEST_INT")
			} else {
				os.Setenv("TEST_INT", tt.envValue)
			}
			result := getEnvInt("TEST_INT", 42)
			if result != tt.want {
				t.Errorf("expected %v, got %v", tt.want, result)
			}
		})
	}
}
