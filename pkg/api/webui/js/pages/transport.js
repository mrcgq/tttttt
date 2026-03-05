/**
 * 传输层设置页面
 */
const TransportPage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">🚀 传输层设置</h2>
  <p style="color:var(--c-text-2);font-size:13px">TLS 验证模式与传输协议</p>
</div>

<div class="card accent-blue">
  <div class="card-header"><h3>🔐 TLS 验证模式</h3></div>
  <div class="card-body">
    <div class="form-group">
      <label>验证模式</label>
      <select id="tls-verify" onchange="TransportPage.updateVerify()">
        <option value="sni-skip" ${App.state.tlsVerifyMode==='sni-skip'?'selected':''}>SNI-Skip — 验证证书链但不验证域名 (域前置推荐)</option>
        <option value="strict" ${App.state.tlsVerifyMode==='strict'?'selected':''}>严格 — 完整验证证书链 + 域名</option>
        <option value="insecure" ${App.state.tlsVerifyMode==='insecure'?'selected':''}>不安全 — 跳过所有验证 (仅测试)</option>
        <option value="pin" ${App.state.tlsVerifyMode==='pin'?'selected':''}>Pin — SHA256 证书固定</option>
      </select>
    </div>
    <div class="form-group" id="tls-pin-group" style="${App.state.tlsVerifyMode==='pin'?'':'display:none'}">
      <label>证书 SHA256 哈希</label>
      <input type="text" id="tls-cert-pin" placeholder="十六进制字符串">
    </div>
    <div class="form-group" id="tls-ca-group">
      <label>自定义 CA 路径 (可选)</label>
      <input type="text" id="tls-custom-ca" placeholder="/path/to/ca.crt">
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>📡 协议对比</h3></div>
  <div class="card-body">
    <div class="grid-3">
      <div class="transport-feature">
        <h4>🔌 WebSocket</h4>
        <ul>
          <li>✅ 二进制支持</li>
          <li>✅ CF Worker 兼容</li>
          <li>✅ Xlink 借力</li>
          <li>✅ 自动 Ping/Pong</li>
          <li>❌ 无多路复用</li>
        </ul>
      </div>
      <div class="transport-feature">
        <h4>📦 HTTP/2 引擎</h4>
        <ul>
          <li>✅ 完整 H2 指纹</li>
          <li>✅ SETTINGS 控制</li>
          <li>✅ PRIORITY 帧</li>
          <li>✅ 多路复用</li>
          <li>⚠️ 使用 WS 底层</li>
        </ul>
      </div>
      <div class="transport-feature">
        <h4>🔗 Raw TCP</h4>
        <ul>
          <li>✅ 最低延迟</li>
          <li>✅ 最简实现</li>
          <li>❌ 需要 VPS</li>
          <li>❌ 易被检测</li>
          <li>❌ 无加密包装</li>
        </ul>
      </div>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>🔗 SOCKS5-Out</h3></div>
  <div class="card-body">
    <div class="transport-feature">
      <h4>🧅 本地 SOCKS5 出站代理</h4>
      <ul>
        <li>✅ Tor 集成</li>
        <li>✅ 本地代理链</li>
        <li>✅ 用户名/密码认证</li>
        <li>⚠️ 额外延迟</li>
      </ul>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>🔄 传输层回退顺序</h3></div>
  <div class="card-body">
    <div class="form-group">
      <label>回退列表 (逗号分隔，按顺序尝试)</label>
      <input type="text" id="transport-fallback-list" value="${App.state.transportFallback}" placeholder="ws,h2,raw">
      <div class="hint">当首选传输层失败时，按此顺序尝试备选方案</div>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-primary" onclick="TransportPage.saveFallback()">保存</button>
  </div>
</div>`;
  },

  updateVerify() {
    const mode = document.getElementById('tls-verify')?.value;
    App.state.tlsVerifyMode = mode;
    const pg = document.getElementById('tls-pin-group');
    if (pg) pg.style.display = mode === 'pin' ? '' : 'none';
    App.log('debug', 'TLS 验证模式: ' + mode);
  },

  saveFallback() {
    const v = document.getElementById('transport-fallback-list')?.value?.trim();
    App.state.transportFallback = v;
    App.toast('回退顺序已保存', 'success');
    App.log('info', '传输回退: ' + v);
  }
};
