# Case Review RAG - 执法大脑

基于 RAG 的执法文书智能审查系统。

## 项目背景

### 为什么做这个

在行政执法领域，案件审查是一项高度依赖专业知识的工作。审查人员需要同时比对大量法律法规条文，耗时长且容易遗漏。传统的关键词搜索无法理解语义上下文，检索精度有限。

本系统针对**执法文书智能审查**场景设计，实现了：
- 自动从文书中提取关键事实与引用法条
- 基于语义理解的精准法律条文召回
- 辅助审查建议与裁量建议生成

### 核心设计决策

#### 为什么选 pgvector 而非其他向量库

| 考量点 | pgvector | Milvus/Qdrant |
|--------|----------|---------------|
| 部署成本 | 零额外依赖，复用现有 PostgreSQL | 需要独立部署维护 |
| 运维复杂度 | 单一数据库管理 | 多组件运维 |
| 混合检索 | 原生支持 SQL + 向量联合查询 | 需要额外开发 |
| 数据一致性 | ACID 事务保障 | 部分不支持事务 |
| 成本 | 零额外基础设施成本 | 需额外服务器资源 |

**最终选择 pgvector**：执法系统对数据一致性要求高，pgvector 依托 PostgreSQL 的成熟生态，在混合检索场景下无需额外的数据同步开发，且部署运维成本最低。

#### 分块策略设计

法律文本的分块直接影响召回效果。本系统采用**条款级分块**策略：

```
正则匹配: 第[一二三四五六七八九十百千]+条
示例: "《行政处罚法》第三十条 ... 第三十一条 ..."
└─> Chunk 1: "《行政处罚法》第三十条 ..."
└─> Chunk 2: "《行政处罚法》第三十一条 ..."
```

每个 chunk 保留完整的法条上下文，包含：
- 法规名称（《行政处罚法》）
- 条款编号（第三十条）
- 条款内容（完整条文）

**优势**：
- 避免跨条款语义污染
- 支持条款级别的精确引用
- 中文数字转阿拉伯数字（第八条 → 8），便于后续精确查询

#### 召回效果评估

当前采用**混合检索 + RRF 融合**：

1. **向量搜索**：基于 bge-m3 生成 1024 维向量，COSINE 相似度
2. **关键词搜索**：SQL LIKE 匹配法条编号
3. **RRF 融合**：

```go
// Reciprocal Rank Fusion
score = 1.0 / (k + rank)
```

评估维度：
- **召回率**：能否覆盖相关法条
- **精确率**：召回结果与查询的相关性
- **引用准确性**：能否精确引用到具体条款

#### 自适应检索策略

系统实现了四种检索策略，可根据查询特征自动选择：

- **Hybrid**：向量 + 关键词 RRF 融合，默认策略
- **Vector**：纯向量搜索，长文本语义理解
- **Keyword**：纯关键词搜索，短查询 + 法条编号
- **Adaptive**：根据查询特征自动选择

```
查询特征 → 策略选择
├─ 长度 < 50 字符 → Keyword（短文本关键词更准）
├─ 包含法条编号（"第X条"）→ Keyword（精确匹配优先）
└─ 其他 → Hybrid（综合最优）
```

### 和 Dify 的对比

Dify 是一个优秀的开源 LLM 应用平台，但针对**执法案件审查**这一垂直场景，存在以下不足：

| 能力 | Dify | 本系统 |
|------|------|--------|
| 法律分块 | 通用文本分块，无法理解法条结构 | 条款级分块，保留法条编号 |
| 法条引用 | RAG 召回后无法精确引用条款 | 提取法条编号，支持精确引用 |
| 多策略检索 | 固定 RAG 流程 | 自适应选择检索策略 |
| 审查工作流 | 通用对话式 | 案件信息提取 → 法条召回 → 审查建议 |
| 部署 | Docker compose / Kubernetes | 单一二进制 + PostgreSQL |

**自研解决的核心问题**：

1. **法条编号丢失问题**：Dify 的通用分块会破坏法条编号，本系统通过正则提取保留完整的条款编号，支持后续精确引用

2. **审查工作流**：不是简单的问答，而是完整的案件审查流程：提取违法行为事实 → 召回相关法条 → 生成审查建议

3. **混合检索优化**：针对法律查询做了专项优化，包含法条编号的查询走关键词优先策略

4. **裁量和审查建议**：基于召回的法条，结合案件事实生成裁量建议，而非泛泛的 RAG 问答

---

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
