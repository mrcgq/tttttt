/**
 * 入站代理页面
 */
const InboundPage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">📥 入站代理</h2>
  <p style="color:var(--c-text-2);font-size:13px">配置本地 SOCKS5 / HTTP CONNECT 代理</p>
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
      <button class="btn btn-primary" onclick="InboundPage.saveSocks5()">💾 保存</button>
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
      </div>
    </div>
    <div class="card-footer">
      <button class="btn btn-primary" onclick="InboundPage.saveHTTP()">💾 保存</button>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>📖 使用说明</h3></div>
  <div class="card-body" style="color:var(--c-text-2);line-height:2">
    <p><strong>浏览器代理设置:</strong></p>
    <p>• SOCKS5: <code style="background:var(--c-bg-3);padding:2px 8px;border-radius:4px">${App.state.socks5Listen}</code></p>
    <p>• HTTP: <code style="background:var(--c-bg-3);padding:2px 8px;border-radius:4px">${App.state.httpListen}</code></p>
    <div class="divider"></div>
    <p><strong>curl 测试:</strong></p>
    <div class="code-block" style="margin-top:8px;white-space:pre-wrap">curl -x socks5://${App.state.socks5Listen} https://httpbin.org/ip
curl -x http://${App.state.httpListen} https://httpbin.org/ip</div>
  </div>
</div>`;
  },

  saveSocks5() {
    App.state.socks5Listen = document.getElementById('ib-socks5-listen')?.value || '127.0.0.1:1080';
    App.state.socks5User = document.getElementById('ib-socks5-user')?.value || '';
    App.state.socks5Pass = document.getElementById('ib-socks5-pass')?.value || '';
    App.toast('SOCKS5 设置已保存', 'success');
    App.log('info', 'SOCKS5 监听: ' + App.state.socks5Listen);
  },

  saveHTTP() {
    App.state.httpListen = document.getElementById('ib-http-listen')?.value || '127.0.0.1:8080';
    App.toast('HTTP 代理设置已保存', 'success');
    App.log('info', 'HTTP 监听: ' + App.state.httpListen);
  }
};
