/**
 * 日志控制台页面
 */
const LogsPage = {
  autoScroll: true,
  filterLevel: 'all',

  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">📋 日志控制台</h2>
  <p style="color:var(--c-text-2);font-size:13px">实时引擎日志 — 共 ${App.state.logs.length} 条</p>
</div>

<div class="card">
  <div class="card-header">
    <h3>🔍 过滤</h3>
    <div class="card-actions">
      <div class="tabs">
        <span class="tab ${this.filterLevel==='all'?'active':''}" onclick="LogsPage.setFilter('all')">全部</span>
        <span class="tab ${this.filterLevel==='info'?'active':''}" onclick="LogsPage.setFilter('info')">INFO</span>
        <span class="tab ${this.filterLevel==='warn'?'active':''}" onclick="LogsPage.setFilter('warn')">WARN</span>
        <span class="tab ${this.filterLevel==='error'?'active':''}" onclick="LogsPage.setFilter('error')">ERROR</span>
        <span class="tab ${this.filterLevel==='debug'?'active':''}" onclick="LogsPage.setFilter('debug')">DEBUG</span>
      </div>
    </div>
  </div>
  <div class="card-body">
    <div class="form-inline" style="margin-bottom:12px">
      <input type="text" id="log-search-input" placeholder="搜索关键字..." style="flex:1" onkeyup="LogsPage.search()">
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header">
    <h3>📜 日志输出</h3>
    <div class="card-actions">
      <button class="btn btn-xs btn-secondary" onclick="LogsPage.toggleScroll()">${this.autoScroll ? '🔒 自动滚动' : '🔓 手动滚动'}</button>
      <button class="btn btn-xs btn-secondary" onclick="LogsPage.exportLogs()">📥 导出</button>
      <button class="btn btn-xs btn-danger" onclick="LogsPage.clearLogs()">🗑️ 清空</button>
    </div>
  </div>
  <div class="card-body">
    <div class="log-console" id="log-output">${this.renderLogs()}</div>
  </div>
</div>`;
  },

  renderLogs() {
    const keyword = '';
    return App.state.logs
      .filter(l => this.filterLevel === 'all' || l.level === this.filterLevel)
      .map(l => `<div class="log-line"><span class="log-time">${l.time}</span><span class="log-level ${l.level}">${l.level.toUpperCase()}</span><span class="log-msg">${this.escapeHtml(l.message)}</span></div>`)
      .join('');
  },

  appendLog(log) {
    const el = document.getElementById('log-output');
    if (!el) return;
    if (this.filterLevel !== 'all' && log.level !== this.filterLevel) return;
    const div = document.createElement('div');
    div.className = 'log-line';
    div.innerHTML = `<span class="log-time">${log.time}</span><span class="log-level ${log.level}">${log.level.toUpperCase()}</span><span class="log-msg">${this.escapeHtml(log.message)}</span>`;
    el.appendChild(div);
    if (this.autoScroll) el.scrollTop = el.scrollHeight;
  },

  setFilter(level) {
    this.filterLevel = level;
    App.renderPage('logs');
  },

  search() {
    const kw = (document.getElementById('log-search-input')?.value || '').toLowerCase();
    const lines = document.querySelectorAll('#log-output .log-line');
    lines.forEach(line => {
      line.style.display = kw === '' || line.textContent.toLowerCase().includes(kw) ? '' : 'none';
    });
  },

  toggleScroll() {
    this.autoScroll = !this.autoScroll;
    App.renderPage('logs');
  },

  clearLogs() {
    App.state.logs = [];
    App.renderPage('logs');
    App.toast('日志已清空', 'info');
  },

  exportLogs() {
    const text = App.state.logs.map(l => `[${l.time}] [${l.level.toUpperCase()}] ${l.message}`).join('\n');
    App.downloadFile('tls-client-logs.txt', text);
    App.toast('日志已导出', 'success');
  },

  escapeHtml(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }
};
