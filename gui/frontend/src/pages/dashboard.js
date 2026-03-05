/**
 * 仪表盘 — 桌面版增加本地引擎启停
 */
const DashboardPage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">🎮 仪表盘</h2>
  <p style="color:var(--c-text-2);font-size:13px">TLS-Client 桌面控制中心</p>
</div>

<div class="engine-row">
  <button class="engine-btn start" onclick="DashboardPage.startLocal()">
    <svg viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>
    启动本地引擎
  </button>
  <button class="engine-btn stop" onclick="DashboardPage.stopLocal()">
    <svg viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
    停止本地引擎
  </button>
</div>

<div class="stats-row">
  <div class="stat-card"><div class="stat-label">引擎状态</div><div class="stat-value green" id="dash-status">待机</div></div>
  <div class="stat-card"><div class="stat-label">运行时间</div><div class="stat-value blue" id="dash-uptime">—</div></div>
  <div class="stat-card"><div class="stat-label">活跃连接</div><div class="stat-value purple" id="dash-conns">0</div></div>
  <div class="stat-card"><div class="stat-label">总流量</div><div class="stat-value" id="dash-bytes">0 B</div></div>
  <div class="stat-card"><div class="stat-label">Goroutines</div><div class="stat-value" id="dash-goroutines">0</div></div>
  <div class="stat-card"><div class="stat-label">内存</div><div class="stat-value" id="dash-mem">0 MB</div></div>
</div>

<div class="grid-2">
  <div class="card accent-blue">
    <div class="card-header"><h3>🎭 当前身份</h3></div>
    <div class="card-body">
      <div class="form-row">
        <div class="form-group"><label>指纹</label><div style="font-size:16px;font-weight:600" id="dash-fp">${App.state.selectedFingerprint}</div></div>
        <div class="form-group"><label>节点</label><div style="font-size:16px;font-weight:600" id="dash-node">${App.state.getActiveNode()?.name||'—'}</div></div>
      </div>
      <div class="form-row">
        <div class="form-group"><label>传输层</label><div style="font-size:16px;font-weight:600" id="dash-transport">${App.state.getActiveNode()?.transport?.toUpperCase()||'—'}</div></div>
        <div class="form-group"><label>TLS 验证</label><div style="font-size:16px;font-weight:600">${App.state.tlsVerifyMode}</div></div>
      </div>
    </div>
  </div>
  <div class="card accent-green">
    <div class="card-header"><h3>⚡ 快捷操作</h3></div>
    <div class="card-body">
      <div class="quick-grid">
        <div class="quick-btn" onclick="DashboardPage.randomFp()"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z"/></svg><span>随机指纹</span></div>
        <div class="quick-btn" onclick="App.navigate('nodes')"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 2v4m0 12v4M2 12h4m12 0h4"/></svg><span>管理节点</span></div>
        <div class="quick-btn" onclick="DashboardPage.openConfig()"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><path d="M14 2v6h6"/></svg><span>打开配置</span></div>
        <div class="quick-btn" onclick="App.navigate('config')"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/></svg><span>导出配置</span></div>
        <div class="quick-btn" onclick="DashboardPage.remoteReload()"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 00-9-9 9.75 9.75 0 00-6.74 2.74L3 8"/><path d="M3 3v5h5"/></svg><span>重载配置</span></div>
        <div class="quick-btn" onclick="App.navigate('logs')"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><path d="M14 2v6h6"/></svg><span>查看日志</span></div>
      </div>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>📊 拨号指标</h3><button class="btn btn-sm btn-secondary" onclick="DashboardPage.refreshMetrics()">刷新</button></div>
  <div class="card-body"><div class="stats-row" id="dash-metrics">
    <div class="stat-card"><div class="stat-label">成功</div><div class="stat-value green">—</div></div>
    <div class="stat-card"><div class="stat-label">失败</div><div class="stat-value red">—</div></div>
    <div class="stat-card"><div class="stat-label">延迟</div><div class="stat-value blue">—</div></div>
    <div class="stat-card"><div class="stat-label">成功率</div><div class="stat-value green">—</div></div>
  </div></div>
</div>`;
  },

  async startLocal() {
    try {
      await API.startLocalEngine('config.yaml');
      App.toast('本地引擎正在启动...', 'info');
      App.log('info', '启动本地引擎');
      const el = document.getElementById('dash-status');
      if (el) { el.textContent = '启动中...'; el.className = 'stat-value yellow'; }
    } catch (e) {
      App.toast('启动失败: ' + e, 'error');
    }
  },

  async stopLocal() {
    try {
      await API.stopLocalEngine();
      App.toast('引擎已停止', 'info');
      App.log('info', '停止本地引擎');
      const el = document.getElementById('dash-status');
      if (el) { el.textContent = '已停止'; el.className = 'stat-value red'; }
    } catch (e) {
      App.toast('停止失败: ' + e, 'error');
    }
  },

  randomFp() {
    const fps = App.state.fingerprints;
    const fp = fps[Math.floor(Math.random() * fps.length)];
    App.state.selectedFingerprint = fp.name;
    const el = document.getElementById('dash-fp');
    if (el) el.textContent = fp.name;
    App.toast('指纹: ' + fp.name, 'info');
    App.log('info', '随机切换: ' + fp.name);
  },

  async openConfig() {
    try {
      const content = await API.openConfigFile();
      if (content) {
        App.toast('配置文件已加载', 'success');
        App.log('info', '打开配置文件');
      }
    } catch (e) {
      App.toast('打开失败: ' + e, 'error');
    }
  },

  async remoteReload() {
    if (!API.connected) { App.toast('请先连接 API', 'warning'); return; }
    try {
      await API.reloadEngine();
      App.toast('配置已重载', 'success');
    } catch (e) { App.toast('重载失败: ' + e, 'error'); }
  },

  updateFromAPI(data) {
    if (!data) return;
    const set = (id, v) => { const e = document.getElementById(id); if (e) e.textContent = v; };
    const el = document.getElementById('dash-status');
    if (el) {
      el.textContent = data.engine_running ? '运行中' : '待机';
      el.className = 'stat-value ' + (data.engine_running ? 'green' : 'yellow');
    }
    set('dash-uptime', data.uptime_human || '—');
    set('dash-conns', data.active_conns ?? 0);
    set('dash-bytes', App.formatBytes(data.total_bytes || 0));
    set('dash-goroutines', data.goroutines ?? 0);
    if (data.memory) set('dash-mem', (data.memory.alloc_mb || 0) + ' MB');
    if (data.current_profile) set('dash-fp', data.current_profile);
    if (data.current_node) set('dash-node', data.current_node);
  },

  async refreshMetrics() {
    if (!API.connected) { App.toast('请先连接 API', 'warning'); return; }
    try {
      const d = await API.getDialMetrics();
      const c = document.getElementById('dash-metrics');
      if (c) c.innerHTML = `
        <div class="stat-card"><div class="stat-label">成功</div><div class="stat-value green">${d.success_count??0}</div></div>
        <div class="stat-card"><div class="stat-label">失败</div><div class="stat-value red">${d.failure_count??0}</div></div>
        <div class="stat-card"><div class="stat-label">延迟</div><div class="stat-value blue">${d.avg_latency_ms??0} ms</div></div>
        <div class="stat-card"><div class="stat-label">成功率</div><div class="stat-value green">${d.success_rate??'—'}</div></div>`;
    } catch (e) { App.toast('获取失败', 'error'); }
  }
};
