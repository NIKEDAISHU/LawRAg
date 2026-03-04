#!/bin/bash

echo "检查所有 HTML 页面的布局类..."
echo ""

for file in web/index.html web/chat.html web/knowledge-modern.html web/projects.html; do
    echo "=== $file ==="
    grep 'class=".*container' "$file" | head -3
    echo ""
done
