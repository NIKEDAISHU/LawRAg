#!/bin/bash
# PostgreSQL 中文分词扩展 zhparser 安装脚本
# 适用于 Docker 容器环境

set -e

echo "=== 开始安装 zhparser 中文分词扩展 ==="

# 使用非交互模式执行
echo "Step 1: 在 Docker 容器中执行安装..."
docker exec law-db bash << 'EOF'

# 2. 安装编译依赖
echo "Step 2: 安装编译依赖..."
apt-get update
apt-get install -y postgresql-server-dev-all build-essential git wget

# 3. 安装 SCWS 中文分词库
echo "Step 3: 安装 SCWS 分词库..."
cd /tmp
wget -q -O - http://www.xunsearch.com/scws/down/scws-1.2.3.tar.bz2 | tar xjf -
cd scws-1.2.3
./configure --prefix=/usr/local
make && make install

# 添加库路径
echo "/usr/local/lib" > /etc/ld.so.conf.d/scws.conf
ldconfig

# 4. 下载并编译 zhparser
echo "Step 4: 编译 zhparser..."
cd /tmp
git clone https://github.com/amutu/zhparser.git
cd zhparser
SCWS_HOME=/usr/local make && make install

# 5. 在数据库中创建扩展
echo "Step 5: 创建数据库扩展..."
psql -U root -d law_kb << 'EOSQL'
-- 创建 zhparser 扩展
CREATE EXTENSION IF NOT EXISTS zhparser;

-- 创建中文全文检索配置
DROP TEXT SEARCH CONFIGURATION IF EXISTS chinese_zh CASCADE;
CREATE TEXT SEARCH CONFIGURATION chinese_zh (PARSER = zhparser);

-- 添加 token 映射
ALTER TEXT SEARCH CONFIGURATION chinese_zh ADD MAPPING FOR n,v,a,i,e,l WITH simple;

-- 测试分词效果
SELECT to_tsvector('chinese_zh', '第八条 经营单位必须建立卫生责任制度');

\echo '✓ zhparser 扩展安装成功！'
EOSQL

exit
EOF

echo "=== zhparser 安装完成 ==="
echo ""
echo "下一步："
echo "1. 修改 migrate.sql 中的 'simple' 为 'chinese_zh'"
echo "2. 重新执行数据库迁移"
echo "3. 重启服务测试"
