const CadencePage = {
  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">⏱️ 时序控制</h2>
  <p style="color:var(--c-text-2);font-size:13px">模拟真人浏览行为的请求节奏</p>
</div>

<div class="card accent-blue">
  <div class="card-header"><h3>🎵 时序模式 (Cadence)</h3></div>
  <div class="card-body">
    <div class="form-row">
      <div class="form-group">
        <label>模式</label>
        <select id="cad-mode" onchange="CadencePage.updateMode()">
          <option value="none" ${App.state.cadenceMode==='none'?'selected':''}>无 — 不添加延迟</option>
          <option value="browsing" ${App.state.cadenceMode==='browsing'?'selected':''}>浏览 — 1~5 秒随机</option>
          <option value="fast" ${App.state.cadenceMode==='fast'?'selected':''}>快速 — 100~500 ms</option>
          <option value="aggressive" ${App.state.cadenceMode==='aggressive'?'selected':''}>激进 — 0~100 ms</option>
          <option value="random" ${App.state.cadenceMode==='random'?'selected':''}>随机 — 0~10 秒</option>
          <option value="custom" ${App.state.cadenceMode==='custom'?'selected':''}>自定义范围</option>
          <option value="sequence" ${App.state.cadenceMode==='sequence'?'selected':''}>序列</option>
        </select>
      </div>
      <div class="form-group">
        <label>Jitter 抖动系数 (0~1)</label>
        <input type="number" id="cad-jitter" value="${App.state.cadenceJitter}" min="0" max="1" step="0.1">
      </div>
    </div>
    <div id="cad-custom" style="${['custom','sequence'].includes(App.state.cadenceMode)?'':'display:none'}">
      <div class="form-row">
        <div class="form-group">
          <label>最小延迟</label>
          <input type="text" id="cad-min" value="${App.state.cadenceMin}" placeholder="100ms">
        </div>
        <div class="form-group">
          <label>最大延迟</label>
          <input type="text" id="cad-max" value="${App.state.cadenceMax}" placeholder="1s">
        </div>
      </div>
      <div class="form-group">
        <label>序列 (仅 sequence 模式，逗号分隔)</label>
        <input type="text" id="cad-seq" value="${App.state.cadenceSeq}" placeholder="500ms,1s,200ms,3s">
      </div>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-primary" onclick="CadencePage.saveCadence()">💾 保存时序设置</button>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>🍪 Cookie 管理</h3></div>
  <div class="card-body">
    <div class="toggle-row">
      <div class="toggle-label">
        <h4>启用 Cookie 管理</h4>
        <p>自动保存和发送 Cookie，模拟真实浏览会话</p>
      </div>
      <label class="toggle"><input type="checkbox" id="cad-cookie" ${App.state.cookieEnabled?'checked':''}><span class="slider"></span></label>
    </div>
    <div class="toggle-row">
      <div class="toggle-label">
        <h4>指纹轮换时清除</h4>
        <p>切换指纹时自动清除所有 Cookie</p>
      </div>
      <label class="toggle"><input type="checkbox" id="cad-cookie-clear" ${App.state.cookieClearOnRotation?'checked':''}><span class="slider"></span></label>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-danger btn-sm" onclick="CadencePage.clearCookies()">🗑️ 清除所有 Cookie</button>
    <button class="btn btn-primary btn-sm" onclick="CadencePage.saveCookieSettings()">💾 保存 Cookie 设置</button>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>↪️ 重定向控制</h3></div>
  <div class="card-body">
    <div class="toggle-row">
      <div class="toggle-label"><h4>跟随重定向</h4><p>自动跟随 HTTP 3xx 重定向</p></div>
      <label class="toggle"><input type="checkbox" id="cad-redirect" ${App.state.followRedirects?'checked':''}><span class="slider"></span></label>
    </div>
    <div class="form-group" style="margin-top:14px">
      <label>最大重定向次数</label>
      <input type="number" id="cad-max-redirect" value="${App.state.maxRedirects}" min="0" max="50">
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-primary" onclick="CadencePage.saveRedirect()">💾 保存重定向设置</button>
  </div>
</div>`;
  },

  updateMode() {
    const mode = document.getElementById('cad-mode')?.value;
    App.state.cadenceMode = mode;
    const custom = document.getElementById('cad-custom');
    if (custom) custom.style.display = ['custom','sequence'].includes(mode) ? '' : 'none';
  },

  saveCadence() {
    App.state.cadenceMode = document.getElementById('cad-mode')?.value || 'none';
    App.state.cadenceJitter = parseFloat(document.getElementById('cad-jitter')?.value) || 0.3;
    App.state.cadenceMin = document.getElementById('cad-min')?.value || '';
    App.state.cadenceMax = document.getElementById('cad-max')?.value || '';
    App.state.cadenceSeq = document.getElementById('cad-seq')?.value || '';
    App.toast('时序设置已保存', 'success');
    App.log('info', '时序模式: ' + App.state.cadenceMode + ', Jitter: ' + App.state.cadenceJitter);
  },

  saveCookieSettings() {
    App.state.cookieEnabled = document.getElementById('cad-cookie')?.checked;
    App.state.cookieClearOnRotation = document.getElementById('cad-cookie-clear')?.checked;
    App.toast('Cookie 设置已保存', 'success');
    App.log('info', 'Cookie: ' + (App.state.cookieEnabled ? '启用' : '禁用'));
  },

  saveRedirect() {
    App.state.followRedirects = document.getElementById('cad-redirect')?.checked;
    App.state.maxRedirects = parseInt(document.getElementById('cad-max-redirect')?.value) || 10;
    App.toast('重定向设置已保存', 'success');
    App.log('info', '重定向: ' + (App.state.followRedirects ? '开启' : '关闭') + ', 最大: ' + App.state.maxRedirects);
  },

  clearCookies() {
    App.toast('所有 Cookie 已清除', 'info');
    App.log('warn', '清除所有 Cookie');
  }
};
