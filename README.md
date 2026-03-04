# Case Review RAG - 执法大脑

基于 RAG 的执法文书智能审查系统。

## 技术栈

- Go 1.25.5
- PostgreSQL + pgvector (向量数据库)
- Gin (Web 框架)
- GORM (ORM)
- OpenAI 兼容 LLM API
- Zap (日志)

## 环境要求

- Go 1.25+
- PostgreSQL 14+
- pgvector 扩展

## 快速开始

### 1. 安装 Go

下载并安装 Go: https://go.dev/dl/

```bash
go version  # 验证安装
```

### 2. 安装 PostgreSQL

**Windows:**
```bash
# 使用 Chocolatey
choco install postgresql

# 或从官网下载安装
# https://www.postgresql.org/download/windows/
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update
sudo apt install postgresql postgresql-contrib
```

**macOS:**
```bash
brew install postgresql@14
brew services start postgresql@14
```

### 3. 安装 pgvector 扩展

**从源码安装:**

```bash
git clone --branch v0.7.4 https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install # may need to be root
```

**或使用包管理器:**

```bash
# Ubuntu 24.04+
sudo apt install postgresql-17-pgvector

# Ubuntu 22.04
sudo apt install postgresql-15-pgvector

# macOS
brew install pgvector
```

### 4. 创建数据库

```bash
# 登录 PostgreSQL
psql -U postgres

# 创建数据库
CREATE DATABASE law_enforcement;

# 启用 pgvector 扩展
\c law_enforcement
CREATE EXTENSION vector;

# 退出
\q
```

或使用 Makefile：

```bash
make db-setup
```

### 5. 配置环境变量

创建 `.env` 文件：

```bash
# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=law_enforcement

# LLM 配置
LLM_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
LLM_API_KEY=your_api_key
LLM_MODEL=deepseek-v3.2
LLM_EMBEDDING_MODEL=bge-m3

# 本地 Embedding (可选)
OLLAMA_BASE_URL=http://localhost:11434

# 向量维度
VECTOR_DIM=1024

# 本地 OCR (可选)
LOCAL_OCR_BASE_URL=http://192.168.90.165:8000
LOCAL_OCR_MODEL=tencent/HunyuanOCR

# 审查配置
REVIEW_STRICTNESS=high
REVIEW_FALLBACK_MODE=true
```

### 6. 安装依赖并运行

```bash
# 安装依赖
go mod download

# 运行服务
go run cmd/api/main.go
```

或使用 Makefile：

```bash
make install-deps
make run
```

服务启动后访问：http://localhost:8888

### 7. 构建

**本地构建:**
```bash
make build
```
编译产物：`bin/law-enforcement-brain`

**Linux 交叉编译:**
```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o bin/law-enforcement-brain-linux-amd64 cmd/api/main.go

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o bin/law-enforcement-brain-linux-arm64 cmd/api/main.go
```

**打包为压缩包:**
```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o bin/law-enforcement-brain cmd/api/main.go
tar -czf law-enforcement-brain-linux-amd64.tar.gz bin/law-enforcement-brain .env web/

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o bin/law-enforcement-brain cmd/api/main.go
tar -czf law-enforcement-brain-linux-arm64.tar.gz bin/law-enforcement-brain .env web/
```

**Linux 部署:**
```bash
# 解压
tar -xzf law-enforcement-brain-linux-amd64.tar.gz

# 运行
./bin/law-enforcement-brain

# 或后台运行
nohup ./bin/law-enforcement-brain > app.log 2>&1 &

# 使用 systemd 管理
sudo vim /etc/systemd/system/law-brain.service
```

systemd 配置示例:
```ini
[Unit]
Description=Law Enforcement Brain
After=network.target postgresql.service

[Service]
Type=simple
User=your_user
WorkingDirectory=/opt/law-enforcement-brain
ExecStart=/opt/law-enforcement-brain/bin/law-enforcement-brain
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable law-brain
sudo systemctl start law-brain
sudo systemctl status law-brain
```

## 数据库表结构

### knowledge_chunks

存储法律知识库的向量嵌入：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | bigint | 主键 |
| project_id | bigint | 项目 ID |
| doc_name | varchar(255) | 文档名称 |
| chunk_content | text | 分块内容 |
| chunk_index | int | 分块索引 |
| embedding | vector(1024) | 向量嵌入 |
| metadata | jsonb | 元数据 |
| created_at | timestamp | 创建时间 |

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/review | 案件审查 |
| POST | /api/v1/upload | 上传法律文档 |
| POST | /api/v1/import-url | 从 URL 导入文档 |
| GET | /api/v1/stats | 文档统计 |
| POST | /api/v1/search | 搜索法律文档 |
| POST | /api/v1/projects | 创建项目 |
| GET | /api/v1/projects | 项目列表 |
| GET | /api/v1/projects/:id | 获取项目 |
| PUT | /api/v1/projects/:id | 更新项目 |
| DELETE | /api/v1/projects/:id | 删除项目 |
| GET | /api/v1/projects/:id/statistics | 项目统计 |
| GET | /api/v1/knowledge/documents | 获取法律文档列表 |
| GET | /api/v1/knowledge/documents/:doc_name/articles | 获取法律条文 |

## Dify 兼容接口

系统提供 Dify 兼容接口供 Java 端调用：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /chat-messages | 对话接口 |
| POST | /v1/chat-messages | 对话接口 (兼容) |

## Makefile 命令

```bash
make build         # 构建应用
make run           # 运行应用
make test          # 运行测试
make clean         # 清理构建产物
make install-deps  # 安装依赖
make db-setup      # 设置数据库
```

## 目录结构

```
.
├── cmd/
│   ├── api/        # API 服务入口
│   └── ingest/     # 数据导入工具
├── internal/
│   ├── adapter/    # 外部适配器
│   ├── api/        # HTTP 处理器
│   ├── core/       # 核心业务逻辑
│   └── domain/     # 领域模型
├── pkg/
│   ├── config/     # 配置管理
│   ├── logger/     # 日志
│   └── splitter/   # 文本分块
├── scripts/        # 脚本
├── test/           # 测试文件
└── web/            # 前端页面
```

## License

MIT
