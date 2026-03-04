// 统一的导航栏组件
function renderClaudeNav(activePage) {
    const currentPage = window.location.pathname.split('/').pop() || 'index.html';
    
    // 如果没有传入 activePage，自动检测当前页面
    let active = activePage;
    if (!active) {
        if (currentPage === 'index.html' || currentPage === '') active = 'home';
        else if (currentPage === 'knowledge-modern.html') active = 'knowledge';
        else if (currentPage === 'projects.html') active = 'projects';
        else if (currentPage === 'chat.html') active = 'chat';
    }
    
    return `
        <nav class="claude-nav">
            <a href="/web/index.html" class="claude-nav-item ${active === 'home' ? 'active' : ''}">
                <span>🏠</span>
                <span>主页</span>
            </a>
            <a href="/web/knowledge-modern.html" class="claude-nav-item ${active === 'knowledge' ? 'active' : ''}">
                <span>📚</span>
                <span>知识库</span>
            </a>
            <a href="/web/projects.html" class="claude-nav-item ${active === 'projects' ? 'active' : ''}">
                <span>⚙️</span>
                <span>项目管理</span>
            </a>
            <a href="/web/chat.html" class="claude-nav-item ${active === 'chat' ? 'active' : ''}">
                <span>💬</span>
                <span>LLM 对话</span>
            </a>
        </nav>
    `;
}

// 自动插入导航栏到页面
function insertNavigation(activePage) {
    const container = document.querySelector('.container');
    if (container) {
        const navHTML = renderClaudeNav(activePage);
        container.insertAdjacentHTML('afterbegin', navHTML);
    }
}

// 页面加载时自动执行
document.addEventListener('DOMContentLoaded', function() {
    // 检查页面是否已有导航栏，如果没有则自动插入
    if (!document.querySelector('.claude-nav')) {
        insertNavigation();
    }
});
