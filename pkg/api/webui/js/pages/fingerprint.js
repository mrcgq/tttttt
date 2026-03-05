/**
 * 指纹实验室页面
 */
const FingerprintPage = {
  filter: 'all',

  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">🎭 指纹实验室</h2>
  <p style="color:var(--c-text-2);font-size:13px">选择和测试浏览器指纹 — 共 ${App.state.fingerprints.length} 种配置</p>
</div>

<div class="card">
  <div class="card-header">
    <h3>🔄 轮换模式</h3>
  </div>
  <div class="card-body">
    <div class="form-row">
      <div class="form-group">
        <label>模式</label>
        <select id="fp-rotation-mode" onchange="FingerprintPage.updateMode()">
          <option value="fixed" ${App.state.rotationMode==='fixed'?'selected':''}>固定 — 始终使用同一指纹</option>
          <option value="random" ${App.state.rotationMode==='random'?'selected':''}>随机 — 每次连接随机</option>
          <option value="per-domain" ${App.state.rotationMode==='per-domain'?'selected':''}>按域名 — 同域名同指纹</option>
          <option value="timed" ${App.state.rotationMode==='timed'?'selected':''}>定时 — 按时间间隔轮换</option>
          <option value="weighted" ${App.state.rotationMode==='weighted'?'selected':''}>加权 — 按权重选择</option>
        </select>
      </div>
      <div class="form-group" id="fp-interval-group" style="${App.state.rotationMode==='timed'?'':'display:none'}">
        <label>轮换间隔</label>
        <input type="text" id="fp-interval" value="5m" placeholder="例如: 5m, 1h">
      </div>
    </div>
  </div>
</div>

<div class="card">
  <div class="card-header">
    <h3>🎯 选择指纹 <span class="badge badge-blue">${App.state.selectedFingerprint}</span></h3>
    <div class="card-actions">
      <div class="tabs">
        <span class="tab ${this.filter==='all'?'active':''}" onclick="FingerprintPage.setFilter('all')">全部</span>
        <span class="tab ${this.filter==='chrome'?'active':''}" onclick="FingerprintPage.setFilter('chrome')">Chrome</span>
        <span class="tab ${this.filter==='firefox'?'active':''}" onclick="FingerprintPage.setFilter('firefox')">Firefox</span>
        <span class="tab ${this.filter==='safari'?'active':''}" onclick="FingerprintPage.setFilter('safari')">Safari</span>
        <span class="tab ${this.filter==='edge'?'active':''}" onclick="FingerprintPage.setFilter('edge')">Edge</span>
      </div>
    </div>
  </div>
  <div class="card-body">
    <div class="fp-grid">${this.renderCards()}</div>
  </div>
</div>

<div class="card accent-green">
  <div class="card-header"><h3>🧪 指纹详情与测试</h3></div>
  <div class="card-body">
    <div id="fp-detail">${this.renderDetail()}</div>
    <div class="btn-group" style="margin-top:16px">
      <button class="btn btn-primary" onclick="FingerprintPage.testFp()">🔬 测试当前指纹</button>
      <button class="btn btn-secondary" onclick="FingerprintPage.randomSelect()">🎲 随机选择</button>
    </div>
    <div id="fp-test-result" style="margin-top:16px;display:none">
      <div class="code-block" id="fp-test-output"></div>
    </div>
  </div>
</div>`;
  },

  renderCards() {
    const fps = this.filter === 'all'
      ? App.state.fingerprints
      : App.state.fingerprints.filter(f => f.browser.toLowerCase() === this.filter);

    return fps.map(fp => `
<div class="fp-card ${fp.name === App.state.selectedFingerprint ? 'selected' : ''}"
     onclick="FingerprintPage.select('${fp.name}')">
  <div class="fp-card-top">
    <div class="fp-card-name">${fp.name}</div>
    <div class="fp-card-tags">
      ${fp.tags.map(t => `<span class="fp-tag ${t}">${t}</span>`).join('')}
    </div>
  </div>
  <div class="fp-card-info">
    <span>🌐 ${fp.browser}</span>
    <span>💻 ${fp.platform}</span>
    <span>v${fp.version}</span>
  </div>
</div>`).join('');
  },

  renderDetail() {
    const fp = App.state.fingerprints.find(f => f.name === App.state.selectedFingerprint);
    if (!fp) return '<p style="color:var(--c-text-2)">未选择指纹</p>';
    return `
<div class="grid-2">
  <div class="form-group"><label>名称</label><div>${fp.name}</div></div>
  <div class="form-group"><label>浏览器</label><div>${fp.browser} ${fp.version}</div></div>
  <div class="form-group"><label>平台</label><div>${fp.platform}</div></div>
  <div class="form-group"><label>标签</label><div>${fp.tags.length ? fp.tags.join(', ') : '无'}</div></div>
</div>`;
  },

  select(name) {
    App.state.selectedFingerprint = name;
    App.renderPage('fingerprint');
    App.log('info', '选择指纹: ' + name);
  },

  setFilter(f) {
    this.filter = f;
    App.renderPage('fingerprint');
  },

  randomSelect() {
    const fps = App.state.fingerprints;
    const fp = fps[Math.floor(Math.random() * fps.length)];
    this.select(fp.name);
    App.toast('随机选择: ' + fp.name, 'info');
  },

  updateMode() {
    const mode = document.getElementById('fp-rotation-mode')?.value;
    App.state.rotationMode = mode;
    const ig = document.getElementById('fp-interval-group');
    if (ig) ig.style.display = mode === 'timed' ? '' : 'none';
    App.log('debug', '轮换模式: ' + mode);
  },

  testFp() {
    const result = document.getElementById('fp-test-result');
    const output = document.getElementById('fp-test-output');
    if (result) result.style.display = 'block';
    if (output) output.textContent = JSON.stringify({
      profile: App.state.selectedFingerprint,
      status: API.connected ? '正在测试...' : '离线模式 — 请连接 API',
      timestamp: new Date().toISOString()
    }, null, 2);
    App.log('info', '测试指纹: ' + App.state.selectedFingerprint);
  }
};
