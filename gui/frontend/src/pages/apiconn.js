const ApiConnPage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">🔗 连接管理</h2>
  <p style="color:var(--c-text-2);font-size:13px">连接到引擎 API 或启动本地引擎</p>
</div>

<!-- 本地引擎管理 (桌面版独有) -->
<div class="card accent-green">
  <div class="card-header">
    <h3>🖥️ 本地引擎</h3>
    <span class="badge ${App.state.localEngineRunning ? 'badge-green' : 'badge-red'}">${App.state.localEngineRunning ? '运行中' : '已停止'}</span>
  </div>
  <div class="card-body">
    <div class="form-group">
      <label>配置文件路径</label>
      <div class="form-inline">
        <input type="text" id="local-config-path" value="config.yaml" style="flex:1">
        <button class="btn btn-secondary" onclick="ApiConnPage.browseConfig()">📂 浏览</button>
      </div>
      <div class="hint">选择 config.yaml 文件路径，tls-client 可执行文件应与 GUI 同目录</div>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-success" onclick="ApiConnPage.startLocal()">▶️ 启动引擎</button>
    <button class="btn btn-danger" onclick="ApiConnPage.stopLocal()">⏹️ 停止引擎</button>
    <button class="btn btn-secondary" onclick="ApiConnPage.checkLocalStatus()">🔍 检查状态</button>
  </div>
</div>

<!-- 远程 API 连接 -->
<div class="card" style="border-left:3px solid ${API.connected ? 'var(--c-green)' : 'var(--c-red)'}">
  <div class="card-header">
    <h3>🌐 远程 API 连接</h3>
    <span class="badge ${API.connected ? 'badge-green' : 'badge-red'}">${API.connected ? '已连接' : '未连接'}</span>
  </div>
  <div class="card-body">
    <div class="form-row">
      <div class="form-group">
        <label>API 地址</label>
        <input type="text" id="api-addr" value="${App.state.apiAddress}">
        <div class="hint">例如: http://127.0.0.1:9090 或远程服务器地址</div>
      </div>
      <div class="form-group">
        <label>认证 Token (可选)</label>
        <input type="password" id="api-tok" value="${App.state.apiToken}" placeholder="Bearer Token">
        <div class="hint">API 配置中的 token 字段值</div>
      </div>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-primary" onclick="ApiConnPage.connect()">🔌 连接</button>
    <button class="btn btn-danger" onclick="ApiConnPage.disconnect()">⛔ 断开</button>
    <button class="btn btn-secondary" onclick="ApiConnPage.test()">🧪 测试连接</button>
  </div>
</div>

<!-- 实时状态 -->
<div class="card">
  <div class="card-header">
    <h3>📊 实时状态</h3>
    <div class="card-actions">
      <button class="btn btn-sm btn-secondary" onclick="ApiConnPage.refresh()">🔄 刷新</button>
      <button class="btn btn-sm btn-secondary" onclick="ApiConnPage.startPoll()">▶️ 自动刷新</button>
      <button class="btn btn-sm btn-secondary" onclick="ApiConnPage.stopPoll()">⏹️ 停止刷新</button>
    </div>
  </div>
  <div class="card-body">
    <div class="code-block" id="api-resp">${API.connected ? '等待刷新...' : '请先连接到引擎 API'}</div>
  </div>
</div>

<!-- API 端点列表 -->
<div class="card">
  <div class="card-header"><h3>📡 API 端点参考</h3></div>
  <div class="card-body">
    <div class="table-wrap">
      <table>
        <thead><tr><th>方法</th><th>端点</th><th>描述</th><th>说明</th></tr></thead>
        <tbody>
          <tr><td><span class="badge badge-green">GET</span></td><td>/api/status</td><td>引擎状态</td><td>运行信息、内存、连接数</td></tr>
          <tr><td><span class="badge badge-green">GET</span></td><td>/api/fingerprints</td><td>指纹列表</td><td>所有已注册指纹配置</td></tr>
          <tr><td><span class="badge badge-green">GET</span></td><td>/api/proxies</td><td>节点健康</td><td>节点延迟、可用性</td></tr>
          <tr><td><span class="badge badge-green">GET</span></td><td>/api/transports</td><td>传输层</td><td>可用传输协议</td></tr>
          <tr><td><span class="badge badge-green">GET</span></td><td>/api/dial-metrics</td><td>拨号指标</td><td>成功率、延迟统计</td></tr>
          <tr><td><span class="badge badge-blue">POST</span></td><td>/api/start</td><td>启动引擎</td><td>启动代理服务</td></tr>
          <tr><td><span class="badge badge-blue">POST</span></td><td>/api/stop</td><td>停止引擎</td><td>停止代理服务</td></tr>
          <tr><td><span class="badge badge-blue">POST</span></td><td>/api/reload</td><td>重载配置</td><td>热重载配置文件</td></tr>
          <tr><td><span class="badge badge-yellow">GET/POST</span></td><td>/api/config</td><td>配置管理</td><td>读取或更新运行配置</td></tr>
          <tr><td><span class="badge badge-green">GET</span></td><td>/health</td><td>健康检查</td><td>简单的存活检测</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</div>`;
  },

  async connect() {
    const addr = document.getElementById('api-addr')?.value || 'http://127.0.0.1:9090';
    const tok = document.getElementById('api-tok')?.value || '';
    App.state.apiAddress = addr;
    App.state.apiToken = tok;
    try {
      const data = await API.connect(addr, tok);
      App.updateApiIndicator(true);
      App.toast('API 连接成功', 'success');
      App.log('info', 'API 已连接: ' + addr);
      App.renderPage('apiconn');
      const el = document.getElementById('api-resp');
      if (el) el.textContent = JSON.stringify(data, null, 2);
    } catch (e) {
      App.updateApiIndicator(false);
      App.toast('连接失败: ' + e, 'error');
      App.log('error', 'API 连接失败: ' + e);
    }
  },

  async disconnect() {
    await API.disconnect();
    API.stopPolling();
    App.updateApiIndicator(false);
    App.renderPage('apiconn');
    App.toast('已断开连接', 'info');
    App.log('info', 'API 已断开');
  },

  async test() {
    try {
      const addr = document.getElementById('api-addr')?.value || App.state.apiAddress;
      const tok = document.getElementById('api-tok')?.value || App.state.apiToken;
      const data = await API.connect(addr, tok);
      App.toast('连接正常!', 'success');
      const el = document.getElementById('api-resp');
      if (el) el.textContent = JSON.stringify(data, null, 2);
      App.updateApiIndicator(true);
      App.renderPage('apiconn');
    } catch (e) {
      App.toast('测试失败: ' + e, 'error');
    }
  },

  async refresh() {
    if (!API.connected) { App.toast('请先连接 API', 'warning'); return; }
    try {
      const data = await API.getStatus();
      const el = document.getElementById('api-resp');
      if (el) el.textContent = JSON.stringify(data, null, 2);
      DashboardPage.updateFromAPI(data);
    } catch (e) {
      App.toast('刷新失败: ' + e, 'error');
    }
  },

  startPoll() {
    if (!API.connected) { App.toast('请先连接 API', 'warning'); return; }
    API.startPolling((data, err) => {
      if (data) {
        const el = document.getElementById('api-resp');
        if (el) el.textContent = JSON.stringify(data, null, 2);
        DashboardPage.updateFromAPI(data);
      }
      if (err) {
        App.log('error', '轮询失败: ' + err);
      }
    }, 5000);
    App.toast('自动刷新已开启 (每5秒)', 'info');
  },

  stopPoll() {
    API.stopPolling();
    App.toast('自动刷新已停止', 'info');
  },

  async startLocal() {
    const path = document.getElementById('local-config-path')?.value || 'config.yaml';
    try {
      await API.startLocalEngine(path);
      App.state.localEngineRunning = true;
      App.toast('本地引擎正在启动...', 'info');
      App.log('info', '启动本地引擎，配置: ' + path);
      App.renderPage('apiconn');
    } catch (e) {
      App.toast('启动失败: ' + e, 'error');
      App.log('error', '本地引擎启动失败: ' + e);
    }
  },

  async stopLocal() {
    try {
      await API.stopLocalEngine();
      App.state.localEngineRunning = false;
      App.toast('本地引擎已停止', 'info');
      App.log('info', '本地引擎已停止');
      App.renderPage('apiconn');
    } catch (e) {
      App.toast('停止失败: ' + e, 'error');
    }
  },

  async checkLocalStatus() {
    try {
      const running = await API.isLocalRunning();
      App.state.localEngineRunning = running;
      App.toast('引擎状态: ' + (running ? '运行中' : '已停止'), running ? 'success' : 'info');
      App.renderPage('apiconn');
    } catch (e) {
      App.toast('检查失败: ' + e, 'error');
    }
  },

  async browseConfig() {
    try {
      const content = await API.openConfigFile();
      if (content) {
        App.toast('配置文件已加载', 'success');
        App.log('info', '加载配置文件');
      }
    } catch (e) {
      App.toast('打开失败: ' + e, 'error');
    }
  }
};
