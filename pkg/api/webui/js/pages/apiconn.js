/**
 * API 连接页面
 */
const ApiConnPage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">🔗 API 连接</h2>
  <p style="color:var(--c-text-2);font-size:13px">连接到正在运行的 TLS-Client 引擎</p>
</div>

<div class="card" style="border-left:3px solid ${API.connected ? 'var(--c-green)' : 'var(--c-red)'}">
  <div class="card-header">
    <h3>状态</h3>
    <span class="badge ${API.connected ? 'badge-green' : 'badge-red'}">${API.connected ? '已连接' : '未连接'}</span>
  </div>
  <div class="card-body">
    <div class="form-row">
      <div class="form-group">
        <label>API 地址</label>
        <input type="text" id="api-addr-input" value="${API.address}">
      </div>
      <div class="form-group">
        <label>认证 Token (可选)</label>
        <input type="password" id="api-token-input" value="${API.token}" placeholder="Bearer token">
      </div>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-primary" onclick="ApiConnPage.connect()">🔌 连接</button>
    <button class="btn btn-danger" onclick="ApiConnPage.disconnect()">⛔ 断开</button>
    <button class="btn btn-secondary" onclick="ApiConnPage.test()">🧪 测试</button>
  </div>
</div>

<div class="card">
  <div class="card-header">
    <h3>📡 API 端点</h3>
  </div>
  <div class="card-body">
    <table>
      <thead><tr><th>方法</th><th>端点</th><th>描述</th></tr></thead>
      <tbody>
        <tr><td><span class="badge badge-green">GET</span></td><td>/api/status</td><td>引擎状态</td></tr>
        <tr><td><span class="badge badge-green">GET</span></td><td>/api/fingerprints</td><td>指纹列表</td></tr>
        <tr><td><span class="badge badge-green">GET</span></td><td>/api/proxies</td><td>节点健康</td></tr>
        <tr><td><span class="badge badge-green">GET</span></td><td>/api/transports</td><td>传输层</td></tr>
        <tr><td><span class="badge badge-green">GET</span></td><td>/api/dial-metrics</td><td>拨号指标</td></tr>
        <tr><td><span class="badge badge-blue">POST</span></td><td>/api/start</td><td>启动引擎</td></tr>
        <tr><td><span class="badge badge-blue">POST</span></td><td>/api/stop</td><td>停止引擎</td></tr>
        <tr><td><span class="badge badge-blue">POST</span></td><td>/api/reload</td><td>重载配置</td></tr>
        <tr><td><span class="badge badge-yellow">GET/POST</span></td><td>/api/config</td><td>读取/更新配置</td></tr>
      </tbody>
    </table>
  </div>
</div>

<div class="card">
  <div class="card-header">
    <h3>📊 实时状态</h3>
    <div class="card-actions">
      <button class="btn btn-sm btn-secondary" onclick="ApiConnPage.refresh()">🔄 刷新</button>
      <button class="btn btn-sm btn-secondary" onclick="ApiConnPage.startPolling()">▶️ 自动刷新</button>
      <button class="btn btn-sm btn-secondary" onclick="ApiConnPage.stopPolling()">⏹️ 停止</button>
    </div>
  </div>
  <div class="card-body">
    <div class="code-block" id="api-response-output">${API.connected ? '等待刷新...' : '请先连接到引擎 API'}</div>
  </div>
</div>`;
  },

  async connect() {
    API.setAddress(document.getElementById('api-addr-input')?.value || 'http://127.0.0.1:9090');
    API.setToken(document.getElementById('api-token-input')?.value || '');
    try {
      const ok = await API.testConnection();
      if (ok) {
        App.updateApiIndicator(true);
        App.toast('API 连接成功', 'success');
        App.log('info', 'API 已连接: ' + API.address);
        App.renderPage('apiconn');
        this.refresh();
      } else {
        throw new Error('连接失败');
      }
    } catch (e) {
      App.updateApiIndicator(false);
      App.toast('连接失败: ' + e.message, 'error');
    }
  },

  disconnect() {
    API.connected = false;
    API.stopPolling();
    App.updateApiIndicator(false);
    App.renderPage('apiconn');
    App.toast('已断开', 'info');
    App.log('info', 'API 已断开');
  },

  async test() {
    try {
      API.setAddress(document.getElementById('api-addr-input')?.value || API.address);
      API.setToken(document.getElementById('api-token-input')?.value || API.token);
      const data = await API.getStatus();
      App.toast('连接正常!', 'success');
      const el = document.getElementById('api-response-output');
      if (el) el.textContent = JSON.stringify(data, null, 2);
    } catch (e) {
      App.toast('测试失败: ' + e.message, 'error');
    }
  },

  async refresh() {
    if (!API.connected) { App.toast('请先连接', 'warning'); return; }
    try {
      const data = await API.getStatus();
      const el = document.getElementById('api-response-output');
      if (el) el.textContent = JSON.stringify(data, null, 2);
      DashboardPage.updateFromAPI(data);
    } catch (e) {
      App.toast('刷新失败', 'error');
    }
  },

  startPolling() {
    if (!API.connected) { App.toast('请先连接', 'warning'); return; }
    API.startPolling((data, err) => {
      if (data) {
        const el = document.getElementById('api-response-output');
        if (el) el.textContent = JSON.stringify(data, null, 2);
        DashboardPage.updateFromAPI(data);
      }
    }, 5000);
    App.toast('自动刷新已开启 (5s)', 'info');
  },

  stopPolling() {
    API.stopPolling();
    App.toast('自动刷新已停止', 'info');
  }
};
