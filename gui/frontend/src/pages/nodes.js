const NodesPage = {
  editingNode: null,

  render() {
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">🌐 节点管理</h2>
  <p style="color:var(--c-text-2);font-size:13px">添加、编辑、删除出站节点 — 支持 CF Worker / VPS / Xlink 借力 / SOCKS5-Out</p>
</div>

<div class="card accent-blue">
  <div class="card-header"><h3>➕ 添加新节点</h3></div>
  <div class="card-body">
    <div class="form-row">
      <div class="form-group">
        <label>节点名称 *</label>
        <input type="text" id="new-node-name" placeholder="例如: my-cf-worker">
      </div>
      <div class="form-group">
        <label>地址 *</label>
        <input type="text" id="new-node-address" placeholder="162.159.19.211:443">
        <div class="hint">IP:Port 格式</div>
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>SNI (域名) *</label>
        <input type="text" id="new-node-sni" placeholder="your-worker.workers.dev">
      </div>
      <div class="form-group">
        <label>传输协议</label>
        <select id="new-node-transport">
          <option value="ws">WebSocket (推荐)</option>
          <option value="h2">HTTP/2</option>
          <option value="raw">Raw TCP</option>
          <option value="socks5-out">SOCKS5-Out</option>
        </select>
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>WebSocket 路径</label>
        <input type="text" id="new-node-ws-path" placeholder="/?token=secret">
      </div>
      <div class="form-group">
        <label>WebSocket Host</label>
        <input type="text" id="new-node-ws-host" placeholder="留空则使用 SNI">
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>H2 路径 (仅 HTTP/2)</label>
        <input type="text" id="new-node-h2-path" placeholder="/h2-tunnel">
      </div>
      <div class="form-group">
        <label>指定指纹 (可选)</label>
        <select id="new-node-fingerprint">
          <option value="">使用全局设置</option>
          ${App.state.fingerprints.map(f => `<option value="${f.name}">${f.name} (${f.browser}/${f.platform})</option>`).join('')}
        </select>
      </div>
    </div>
    <div class="section-title">🧦 SOCKS5-Out 配置</div>
    <div class="form-row">
      <div class="form-group">
        <label>SOCKS5-Out 代理地址</label>
        <input type="text" id="new-node-socks5out" placeholder="127.0.0.1:9050">
      </div>
      <div class="form-group">
        <label>SOCKS5-Out 用户名</label>
        <input type="text" id="new-node-socks5out-user" placeholder="可选">
      </div>
      <div class="form-group">
        <label>SOCKS5-Out 密码</label>
        <input type="password" id="new-node-socks5out-pass" placeholder="可选">
      </div>
    </div>
    <div class="section-title">🔗 Xlink 借力配置 (可选)</div>
    <div class="form-row">
      <div class="form-group">
        <label>远程 SOCKS5</label>
        <input type="text" id="new-node-remote-socks5" placeholder="user:pass@proxy.example.com:1080">
        <div class="hint">Worker 使用的远程 SOCKS5 代理</div>
      </div>
      <div class="form-group">
        <label>Fallback</label>
        <input type="text" id="new-node-remote-fallback" placeholder="proxyip.example.net:443">
        <div class="hint">Worker 直连失败时的备用地址</div>
      </div>
    </div>
    <div class="section-title">🔀 传输层 Fallback (可选)</div>
    <div class="form-row">
      <div class="form-group" style="flex:1">
        <label>Fallback 协议列表</label>
        <input type="text" id="new-node-transport-fallback" placeholder="ws,h2,raw (逗号分隔)">
        <div class="hint">当主传输协议失败时尝试的备用协议</div>
      </div>
    </div>
    <div class="section-title">🔁 重试与连接池</div>
    <div class="form-row">
      <div class="form-group">
        <label>最大重试</label>
        <input type="number" id="new-node-retry-max" value="3" min="1" max="10">
      </div>
      <div class="form-group">
        <label>基础延迟</label>
        <input type="text" id="new-node-retry-base" value="500ms">
      </div>
      <div class="form-group">
        <label>最大延迟</label>
        <input type="text" id="new-node-retry-maxdelay" value="5s">
      </div>
      <div class="form-group">
        <label>Jitter</label>
        <input type="number" id="new-node-retry-jitter" value="0.3" min="0" max="1" step="0.1">
      </div>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>最大空闲连接</label>
        <input type="number" id="new-node-pool-maxidle" value="10" min="1">
      </div>
      <div class="form-group">
        <label>每 Key 最大连接</label>
        <input type="number" id="new-node-pool-perkey" value="5" min="1">
      </div>
      <div class="form-group">
        <label>空闲超时</label>
        <input type="text" id="new-node-pool-idle-timeout" value="120s">
      </div>
      <div class="form-group">
        <label>最大生命周期</label>
        <input type="text" id="new-node-pool-max-life" value="10m">
      </div>
    </div>
  </div>
  <div class="card-footer">
    <button class="btn btn-primary" onclick="NodesPage.addNode()">✅ 添加节点</button>
    <button class="btn btn-secondary" onclick="NodesPage.resetAddForm()">🔄 重置</button>
  </div>
</div>

<div class="card">
  <div class="card-header">
    <h3>📋 节点列表 <span class="badge badge-blue">${App.state.nodes.length}</span></h3>
    <div class="card-actions">
      <button class="btn btn-sm btn-primary" onclick="NodesPage.syncFromEngine()">📡 从引擎同步</button>
      <button class="btn btn-sm btn-secondary" onclick="NodesPage.collapseAll()">全部折叠</button>
    </div>
  </div>
  <div class="card-body" id="nodes-list-container">
    ${this.renderNodeList()}
  </div>
</div>`;
  },

  renderNodeList() {
    if (App.state.nodes.length === 0) {
      return `<div class="empty-state"><h4>暂无节点</h4><p>请在上方添加第一个节点，或点击"从引擎同步"获取现有配置</p></div>`;
    }
    return App.state.nodes.map((n, i) => this.renderNodeItem(n, i)).join('');
  },

  renderNodeItem(node, index) {
    const isActive = node.active;
    const isEditing = this.editingNode === node.name;

    return `
<div class="node-item ${isActive ? 'node-active' : ''} ${isEditing ? 'node-editing' : ''}" id="node-${index}">
  <div class="node-top">
    <div class="node-name">
      ${node.name}
      ${isActive ? '<span class="badge badge-green">活跃</span>' : ''}
      ${node.transport === 'socks5-out' ? '<span class="badge badge-orange">SOCKS5-Out</span>' : ''}
      ${node.remoteProxy?.socks5 ? '<span class="badge badge-purple">Xlink</span>' : ''}
      ${node.transportFallback?.length ? '<span class="badge badge-blue">Fallback</span>' : ''}
    </div>
    <div class="btn-group">
      ${isEditing
        ? `<button class="btn btn-xs btn-warning" onclick="NodesPage.cancelEdit(${index})">取消</button>`
        : `<button class="btn btn-xs btn-secondary" onclick="NodesPage.startEdit(${index})">✏️ 修改</button>`}
      ${!isActive ? `<button class="btn btn-xs btn-success" onclick="NodesPage.activate(${index})">激活</button>` : ''}
      <button class="btn btn-xs btn-danger" onclick="NodesPage.confirmDelete(${index})">🗑️ 删除</button>
    </div>
  </div>
  <div class="node-details">
    <span class="node-detail">📍 ${node.address}</span>
    <span class="node-detail">🔒 ${node.sni}</span>
    <span class="node-detail">🚀 ${node.transport.toUpperCase()}</span>
    ${node.fingerprint ? `<span class="node-detail">🎭 ${node.fingerprint}</span>` : ''}
    ${node.transportOpts?.wsPath && node.transport === 'ws' ? `<span class="node-detail">📂 ${node.transportOpts.wsPath}</span>` : ''}
    ${node.transportOpts?.h2Path && node.transport === 'h2' ? `<span class="node-detail">📂 H2:${node.transportOpts.h2Path}</span>` : ''}
    ${node.transportOpts?.socks5Addr && node.transport === 'socks5-out' ? `<span class="node-detail">🧦 ${node.transportOpts.socks5Addr}</span>` : ''}
    ${node.transportFallback?.length ? `<span class="node-detail">🔀 fallback: ${node.transportFallback.join(',')}</span>` : ''}
  </div>
  ${isEditing ? this.renderEditForm(node, index) : ''}
  <div class="node-actions-bar" id="node-delete-confirm-${index}" style="display:none">
    <div class="confirm-bar">
      <span>⚠️ 确定要删除节点 "${node.name}" 吗？此操作不可撤销。</span>
      <button class="btn btn-xs btn-danger" onclick="NodesPage.deleteNode(${index})">确认删除</button>
      <button class="btn btn-xs btn-secondary" onclick="NodesPage.cancelDelete(${index})">取消</button>
    </div>
  </div>
</div>`;
  },

  renderEditForm(node, index) {
    const opts = node.transportOpts || {};
    const rp = node.remoteProxy || {};
    const retry = node.retry || {};
    const pool = node.pool || {};
    const fallback = node.transportFallback || [];

    return `
<div class="node-edit-form visible" id="node-edit-${index}">
  <div class="form-row">
    <div class="form-group">
      <label>节点名称</label>
      <input type="text" id="edit-name-${index}" value="${node.name}">
    </div>
    <div class="form-group">
      <label>地址</label>
      <input type="text" id="edit-address-${index}" value="${node.address}">
    </div>
  </div>
  <div class="form-row">
    <div class="form-group">
      <label>SNI</label>
      <input type="text" id="edit-sni-${index}" value="${node.sni}">
    </div>
    <div class="form-group">
      <label>传输协议</label>
      <select id="edit-transport-${index}">
        <option value="ws" ${node.transport==='ws'?'selected':''}>WebSocket</option>
        <option value="h2" ${node.transport==='h2'?'selected':''}>HTTP/2</option>
        <option value="raw" ${node.transport==='raw'?'selected':''}>Raw</option>
        <option value="socks5-out" ${node.transport==='socks5-out'?'selected':''}>SOCKS5-Out</option>
      </select>
    </div>
  </div>
  <div class="form-row">
    <div class="form-group">
      <label>WS 路径</label>
      <input type="text" id="edit-wspath-${index}" value="${opts.wsPath || ''}">
    </div>
    <div class="form-group">
      <label>WS Host</label>
      <input type="text" id="edit-wshost-${index}" value="${opts.wsHost || ''}">
    </div>
    <div class="form-group">
      <label>H2 路径</label>
      <input type="text" id="edit-h2path-${index}" value="${opts.h2Path || ''}">
    </div>
  </div>
  <div class="form-row">
    <div class="form-group">
      <label>指纹</label>
      <select id="edit-fp-${index}">
        <option value="">全局</option>
        ${App.state.fingerprints.map(f => `<option value="${f.name}" ${node.fingerprint===f.name?'selected':''}>${f.name}</option>`).join('')}
      </select>
    </div>
  </div>
  <div class="section-title">🧦 SOCKS5-Out</div>
  <div class="form-row">
    <div class="form-group">
      <label>SOCKS5-Out 地址</label>
      <input type="text" id="edit-s5out-${index}" value="${opts.socks5Addr || ''}">
    </div>
    <div class="form-group">
      <label>SOCKS5-Out 用户名</label>
      <input type="text" id="edit-s5out-user-${index}" value="${opts.socks5Username || ''}">
    </div>
    <div class="form-group">
      <label>SOCKS5-Out 密码</label>
      <input type="password" id="edit-s5out-pass-${index}" value="${opts.socks5Password || ''}">
    </div>
  </div>
  <div class="section-title">🔗 Xlink 借力</div>
  <div class="form-row">
    <div class="form-group">
      <label>远程 SOCKS5</label>
      <input type="text" id="edit-rs5-${index}" value="${rp.socks5 || ''}">
    </div>
    <div class="form-group">
      <label>Fallback</label>
      <input type="text" id="edit-rfb-${index}" value="${rp.fallback || ''}">
    </div>
  </div>
  <div class="section-title">🔀 传输层 Fallback</div>
  <div class="form-row">
    <div class="form-group" style="flex:1">
      <label>Fallback 协议列表</label>
      <input type="text" id="edit-tfb-${index}" value="${fallback.join(',')}" placeholder="ws,h2,raw">
    </div>
  </div>
  <div class="section-title">🔁 重试</div>
  <div class="form-row">
    <div class="form-group">
      <label>最大重试</label>
      <input type="number" id="edit-retry-max-${index}" value="${retry.maxAttempts || 3}">
    </div>
    <div class="form-group">
      <label>基础延迟</label>
      <input type="text" id="edit-retry-base-${index}" value="${retry.baseDelay || '500ms'}">
    </div>
    <div class="form-group">
      <label>最大延迟</label>
      <input type="text" id="edit-retry-maxd-${index}" value="${retry.maxDelay || '5s'}">
    </div>
    <div class="form-group">
      <label>Jitter</label>
      <input type="number" id="edit-jitter-${index}" value="${retry.jitter || 0.3}" step="0.1" min="0" max="1">
    </div>
  </div>
  <div class="section-title">🏊 连接池</div>
  <div class="form-row">
    <div class="form-group">
      <label>最大空闲</label>
      <input type="number" id="edit-pool-idle-${index}" value="${pool.maxIdle || 10}">
    </div>
    <div class="form-group">
      <label>每 Key 最大</label>
      <input type="number" id="edit-pool-perkey-${index}" value="${pool.maxPerKey || 5}">
    </div>
    <div class="form-group">
      <label>空闲超时</label>
      <input type="text" id="edit-pool-timeout-${index}" value="${pool.idleTimeout || '120s'}">
    </div>
    <div class="form-group">
      <label>最大生命周期</label>
      <input type="text" id="edit-pool-life-${index}" value="${pool.maxLifetime || '10m'}">
    </div>
  </div>
  <div class="card-footer" style="margin-top:12px">
    <button class="btn btn-primary" onclick="NodesPage.saveEdit(${index})">💾 保存修改</button>
    <button class="btn btn-secondary" onclick="NodesPage.cancelEdit(${index})">取消</button>
  </div>
</div>`;
  },

  async addNode() {
    const name = document.getElementById('new-node-name')?.value?.trim();
    const address = document.getElementById('new-node-address')?.value?.trim();
    const sni = document.getElementById('new-node-sni')?.value?.trim();

    if (!name) { App.toast('节点名称不能为空', 'error'); return; }
    if (!address) { App.toast('地址不能为空', 'error'); return; }
    if (!sni) { App.toast('SNI 不能为空', 'error'); return; }
    if (App.state.nodes.find(n => n.name === name)) { App.toast('节点名称已存在', 'error'); return; }

    const tfbStr = document.getElementById('new-node-transport-fallback')?.value?.trim() || '';
    const transportFallback = tfbStr ? tfbStr.split(',').map(s => s.trim()).filter(s => s) : [];

    const node = {
      name,
      address,
      sni,
      transport: document.getElementById('new-node-transport')?.value || 'ws',
      fingerprint: document.getElementById('new-node-fingerprint')?.value || '',
      active: App.state.nodes.length === 0,
      transportOpts: {
        wsPath: document.getElementById('new-node-ws-path')?.value || '/',
        wsHost: document.getElementById('new-node-ws-host')?.value || '',
        h2Path: document.getElementById('new-node-h2-path')?.value || '',
        socks5Addr: document.getElementById('new-node-socks5out')?.value || '',
        socks5Username: document.getElementById('new-node-socks5out-user')?.value || '',
        socks5Password: document.getElementById('new-node-socks5out-pass')?.value || '',
        wsHeaders: {},
      },
      remoteProxy: {
        socks5: document.getElementById('new-node-remote-socks5')?.value || '',
        fallback: document.getElementById('new-node-remote-fallback')?.value || '',
      },
      retry: {
        maxAttempts: parseInt(document.getElementById('new-node-retry-max')?.value) || 3,
        baseDelay: document.getElementById('new-node-retry-base')?.value || '500ms',
        maxDelay: document.getElementById('new-node-retry-maxdelay')?.value || '5s',
        jitter: parseFloat(document.getElementById('new-node-retry-jitter')?.value) || 0.3,
      },
      pool: {
        maxIdle: parseInt(document.getElementById('new-node-pool-maxidle')?.value) || 10,
        maxPerKey: parseInt(document.getElementById('new-node-pool-perkey')?.value) || 5,
        idleTimeout: document.getElementById('new-node-pool-idle-timeout')?.value || '120s',
        maxLifetime: document.getElementById('new-node-pool-max-life')?.value || '10m',
      },
      transportFallback: transportFallback,
    };

    App.state.nodes.push(node);

    const success = await App.pushConfigToEngine();

    if (success) {
      this.resetAddForm();
      App.renderPage('nodes');
      App.log('info', '添加节点: ' + name);
    } else {
      App.state.nodes.pop();
      App.toast('添加节点失败，已回滚', 'error');
    }
  },

  resetAddForm() {
    ['new-node-name','new-node-address','new-node-sni','new-node-ws-path','new-node-ws-host',
     'new-node-h2-path','new-node-socks5out','new-node-socks5out-user','new-node-socks5out-pass',
     'new-node-remote-socks5','new-node-remote-fallback','new-node-transport-fallback'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.value = '';
    });
  },

  startEdit(index) {
    this.editingNode = App.state.nodes[index].name;
    App.renderPage('nodes');
  },

  cancelEdit() {
    this.editingNode = null;
    App.renderPage('nodes');
  },

  async saveEdit(index) {
    const node = App.state.nodes[index];
    const oldNode = JSON.parse(JSON.stringify(node));

    const g = id => document.getElementById(id)?.value?.trim() || '';

    node.name = g(`edit-name-${index}`) || node.name;
    node.address = g(`edit-address-${index}`) || node.address;
    node.sni = g(`edit-sni-${index}`) || node.sni;
    node.transport = g(`edit-transport-${index}`) || node.transport;
    node.fingerprint = g(`edit-fp-${index}`);

    if (!node.transportOpts) node.transportOpts = {};
    node.transportOpts.wsPath = g(`edit-wspath-${index}`);
    node.transportOpts.wsHost = g(`edit-wshost-${index}`);
    node.transportOpts.h2Path = g(`edit-h2path-${index}`);
    node.transportOpts.socks5Addr = g(`edit-s5out-${index}`);
    node.transportOpts.socks5Username = g(`edit-s5out-user-${index}`);
    node.transportOpts.socks5Password = g(`edit-s5out-pass-${index}`);

    if (!node.remoteProxy) node.remoteProxy = {};
    node.remoteProxy.socks5 = g(`edit-rs5-${index}`);
    node.remoteProxy.fallback = g(`edit-rfb-${index}`);

    const tfbStr = g(`edit-tfb-${index}`);
    node.transportFallback = tfbStr ? tfbStr.split(',').map(s => s.trim()).filter(s => s) : [];

    if (!node.retry) node.retry = {};
    node.retry.maxAttempts = parseInt(g(`edit-retry-max-${index}`)) || 3;
    node.retry.baseDelay = g(`edit-retry-base-${index}`) || '500ms';
    node.retry.maxDelay = g(`edit-retry-maxd-${index}`) || '5s';
    node.retry.jitter = parseFloat(g(`edit-jitter-${index}`)) || 0.3;

    if (!node.pool) node.pool = {};
    node.pool.maxIdle = parseInt(g(`edit-pool-idle-${index}`)) || 10;
    node.pool.maxPerKey = parseInt(g(`edit-pool-perkey-${index}`)) || 5;
    node.pool.idleTimeout = g(`edit-pool-timeout-${index}`) || '120s';
    node.pool.maxLifetime = g(`edit-pool-life-${index}`) || '10m';
    
	// gui/frontend/src/pages/nodes.js 中的 saveEdit 函数末尾
    const success = await App.pushConfigToEngine();

    if (success) {
      this.editingNode = null;
      // 🚀 修复：前端稍等 1 秒，等后端把旧连接杀干净、新连接建好，再刷新界面
      setTimeout(() => {
        App.renderPage('nodes');
        App.log('info', '修改节点: ' + node.name);
      }, 1000); 
    } else {
      App.state.nodes[index] = oldNode;
      App.toast('保存失败，已回滚', 'error');
    }

  async activate(index) {
    const oldActiveIndex = App.state.nodes.findIndex(n => n.active);

    App.state.nodes.forEach((n, i) => n.active = (i === index));

    const success = await App.pushConfigToEngine();

    if (success) {
      App.renderPage('nodes');
      App.log('info', '激活节点: ' + App.state.nodes[index].name);
    } else {
      App.state.nodes.forEach((n, i) => n.active = (i === oldActiveIndex));
      App.toast('激活失败，已回滚', 'error');
    }
  },

  confirmDelete(index) {
    document.getElementById(`node-delete-confirm-${index}`).style.display = 'block';
  },

  cancelDelete(index) {
    document.getElementById(`node-delete-confirm-${index}`).style.display = 'none';
  },

  async deleteNode(index) {
    const name = App.state.nodes[index].name;
    const deletedNode = App.state.nodes.splice(index, 1)[0];

    const success = await App.pushConfigToEngine();

    if (success) {
      App.renderPage('nodes');
      App.log('warn', '删除节点: ' + name);
    } else {
      App.state.nodes.splice(index, 0, deletedNode);
      App.toast('删除失败，已回滚', 'error');
    }
  },

  collapseAll() {
    this.editingNode = null;
    App.renderPage('nodes');
  },

  async syncFromEngine() {
    if (!API.connected) {
      App.toast('请先连接到引擎 API', 'warning');
      return;
    }

    try {
      await App.fetchAndApplyConfig();
      App.renderPage('nodes');
      App.toast('已从引擎同步 ' + App.state.nodes.length + ' 个节点', 'success');
    } catch (e) {
      App.toast('同步失败: ' + e, 'error');
    }
  }
};



