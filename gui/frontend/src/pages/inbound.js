const InboundPage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">📥 入站代理</h2>
  <p style="color:var(--c-text-2);font-size:13px">配置本地 SOCKS5 / HTTP CONNECT 代理服务器</p>
</div>

<div class="grid-2">
  <div class="card accent-blue">
    <div class="card-header">
      <h3>🧦 SOCKS5 代理</h3>
      <label class="toggle"><input type="checkbox" id="ib-socks5-on" checked><span class="slider"></span></label>
    </div>
    <div class="card-body">
      <div class="form-group">
        <label>监听地址</label>
        <input type="text" id="ib-socks5-listen" value="${App.state.socks5Listen}">
        <div class="hint">格式: IP:端口，例如 127.0.0.1:1080 或 0.0.0.0:1080</div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>用户名 (可选)</label>
          <input type="text" id="ib-socks5-user" value="${App.state.socks5User}" placeholder="留空则无认证">
        </div>
        <div class="form-group">
          <label>密码 (可选)</label>
          <input type="password" id="ib-socks5-pass" value="${App.state.socks5Pass}" placeholder="留空则无认证">
        </div>
      </div>
    </div>
    <div class="card-footer">
      <button class="btn btn-primary" onclick="InboundPage.saveSocks5()">💾 保存并应用</button>
    </div>
  </div>

  <div class="card accent-green">
    <div class="card-header">
      <h3>🌐 HTTP CONNECT 代理</h3>
      <label class="toggle"><input type="checkbox" id="ib-http-on" checked><span class="slider"></span></label>
    </div>
    <div class="card-body">
      <div class="form-group">
        <label>监听地址</label>
        <input type="text" id="ib-http-listen" value="${App.state.httpListen}">
        <div class="hint">格式: IP:端口，例如 127.0.0.1:8080</div>
      </div>
    </div>
    <div class="card-footer">
      <button class="btn btn-primary" onclick="InboundPage.saveHTTP()">💾 保存并应用</button>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header">
    <h3>⚡ 快捷操作</h3>
  </div>
  <div class="card-body">
    <div class="btn-group">
      <button class="btn btn-secondary" onclick="InboundPage.syncFromEngine()">📡 从引擎同步</button>
      <button class="btn btn-warning" onclick="InboundPage.saveAll()">💾 保存全部并重载</button>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>📖 使用说明</h3></div>
  <div class="card-body" style="color:var(--c-text-2);line-height:2">
    <p><strong>当前配置:</strong></p>
    <p>• SOCKS5 代理: <code style="background:var(--c-bg-3);padding:2px 8px;border-radius:4px">${App.state.socks5Listen}</code></p>
    <p>• HTTP 代理: <code style="background:var(--c-bg-3);padding:2px 8px;border-radius:4px">${App.state.httpListen}</code></p>
    <div class="divider"></div>
    <p><strong>命令行测试:</strong></p>
    <div class="code-block" style="margin-top:8px;white-space:pre-wrap"># SOCKS5
curl -x socks5://${App.state.socks5Listen} https://httpbin.org/ip

# HTTP CONNECT
curl -x http://${App.state.httpListen} https://httpbin.org/ip

# 带认证的 SOCKS5
curl -x socks5://user:pass@${App.state.socks5Listen} https://httpbin.org/ip</div>
  </div>
</div>`;
  },

  // ================================================================
  // 保存 SOCKS5 设置
  // ================================================================
  async saveSocks5() {
    const newListen = document.getElementById('ib-socks5-listen')?.value?.trim() || '127.0.0.1:1080';
    const newUser = document.getElementById('ib-socks5-user')?.value?.trim() || '';
    const newPass = document.getElementById('ib-socks5-pass')?.value || '';

    // 验证格式
    if (!newListen.includes(':')) {
      App.toast('监听地址格式错误，应为 IP:端口', 'error');
      return;
    }

    const oldListen = App.state.socks5Listen;
    const oldUser = App.state.socks5User;
    const oldPass = App.state.socks5Pass;

    App.state.socks5Listen = newListen;
    App.state.socks5User = newUser;
    App.state.socks5Pass = newPass;

    // 推送配置到核心引擎
    const success = await App.pushConfigToEngine();

    if (success) {
      App.renderPage('inbound');
      App.log('info', 'SOCKS5 监听地址已更新: ' + newListen);
    } else {
      // 回滚
      App.state.socks5Listen = oldListen;
      App.state.socks5User = oldUser;
      App.state.socks5Pass = oldPass;
      App.toast('保存失败，已回滚', 'error');
    }
  },

  // ================================================================
  // 保存 HTTP 设置
  // ================================================================
  async saveHTTP() {
    const newListen = document.getElementById('ib-http-listen')?.value?.trim() || '127.0.0.1:8080';

    // 验证格式
    if (!newListen.includes(':')) {
      App.toast('监听地址格式错误，应为 IP:端口', 'error');
      return;
    }

    const oldListen = App.state.httpListen;

    App.state.httpListen = newListen;

    // 推送配置到核心引擎
    const success = await App.pushConfigToEngine();

    if (success) {
      App.renderPage('inbound');
      App.log('info', 'HTTP 监听地址已更新: ' + newListen);
    } else {
      // 回滚
      App.state.httpListen = oldListen;
      App.toast('保存失败，已回滚', 'error');
    }
  },

  // ================================================================
  // 保存全部设置
  // ================================================================
  async saveAll() {
    const socks5Listen = document.getElementById('ib-socks5-listen')?.value?.trim() || '127.0.0.1:1080';
    const socks5User = document.getElementById('ib-socks5-user')?.value?.trim() || '';
    const socks5Pass = document.getElementById('ib-socks5-pass')?.value || '';
    const httpListen = document.getElementById('ib-http-listen')?.value?.trim() || '127.0.0.1:8080';

    // 验证格式
    if (!socks5Listen.includes(':') || !httpListen.includes(':')) {
      App.toast('监听地址格式错误，应为 IP:端口', 'error');
      return;
    }

    const oldState = {
      socks5Listen: App.state.socks5Listen,
      socks5User: App.state.socks5User,
      socks5Pass: App.state.socks5Pass,
      httpListen: App.state.httpListen,
    };

    App.state.socks5Listen = socks5Listen;
    App.state.socks5User = socks5User;
    App.state.socks5Pass = socks5Pass;
    App.state.httpListen = httpListen;

    // 推送配置到核心引擎
    const success = await App.pushConfigToEngine();

    if (success) {
      App.renderPage('inbound');
      App.log('info', '入站代理配置已更新');
    } else {
      // 回滚
      App.state.socks5Listen = oldState.socks5Listen;
      App.state.socks5User = oldState.socks5User;
      App.state.socks5Pass = oldState.socks5Pass;
      App.state.httpListen = oldState.httpListen;
      App.toast('保存失败，已回滚', 'error');
    }
  },

  // ================================================================
  // 从核心引擎同步配置
  // ================================================================
  async syncFromEngine() {
    if (!API.connected) {
      App.toast('请先连接到引擎 API', 'warning');
      return;
    }

    try {
      await App.fetchAndApplyConfig();
      App.renderPage('inbound');
      App.toast('已从引擎同步入站代理配置', 'success');
    } catch (e) {
      App.toast('同步失败: ' + e, 'error');
    }
  }
};
