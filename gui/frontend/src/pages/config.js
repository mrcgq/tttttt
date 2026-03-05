const ConfigPage = {
  render() {
    const yaml = this.generateYAML();
    return `
<div class="page-header" style="margin-bottom:20px">
  <h2 style="font-size:22px;margin-bottom:4px">📤 配置导出</h2>
  <p style="color:var(--c-text-2);font-size:13px">根据当前面板设置生成 config.yaml</p>
</div>

<div class="card">
  <div class="card-header">
    <h3>📄 配置预览</h3>
    <div class="card-actions">
      <button class="btn btn-sm btn-primary" onclick="ConfigPage.copy()">📋 复制</button>
      <button class="btn btn-sm btn-success" onclick="ConfigPage.download()">💾 保存文件</button>
      <button class="btn btn-sm btn-secondary" onclick="ConfigPage.refresh()">🔄 刷新</button>
    </div>
  </div>
  <div class="card-body">
    <div class="code-block" id="config-yaml-output">${this.escapeHtml(yaml)}</div>
  </div>
</div>

<div class="card">
  <div class="card-header"><h3>📖 使用方法</h3></div>
  <div class="card-body" style="color:var(--c-text-2);line-height:2">
    <p>1. 点击「保存文件」选择保存路径</p>
    <p>2. 运行引擎:</p>
    <div class="code-block" style="margin:8px 0">./tls-client run -c config.yaml</div>
    <p>3. 或在「连接管理」页面选择配置文件直接启动本地引擎</p>
    <p>4. 也可通过 API 动态更新配置</p>
  </div>
</div>`;
  },

  generateYAML() {
    const s = App.state;

    let yaml = `# TLS-Client Phase 3.5 配置文件
# 由桌面控制面板生成于 ${new Date().toLocaleString('zh-CN')}

global:
  log_level: "debug"
  log_output: "stderr"

inbound:
  socks5:
    listen: "${s.socks5Listen}"
    username: "${s.socks5User}"
    password: "${s.socks5Pass}"
  http:
    listen: "${s.httpListen}"

fingerprint:
  rotation:
    mode: "${s.rotationMode}"
    profile: "${s.selectedFingerprint}"
    profiles:
${s.fingerprints.map(f => '      - "' + f.name + '"').join('\n')}

tls:
  verify_mode: "${s.tlsVerifyMode}"

client_behavior:
  cadence:
    mode: "${s.cadenceMode}"
    jitter: ${s.cadenceJitter}${s.cadenceMin ? '\n    min_delay: "' + s.cadenceMin + '"' : ''}${s.cadenceMax ? '\n    max_delay: "' + s.cadenceMax + '"' : ''}${s.cadenceSeq ? '\n    sequence:\n' + s.cadenceSeq.split(',').map(v => '      - "' + v.trim() + '"').join('\n') : ''}
  cookies:
    enabled: ${s.cookieEnabled}
    clear_on_rotation: ${s.cookieClearOnRotation}
  follow_redirects: ${s.followRedirects}
  max_redirects: ${s.maxRedirects}

nodes:
`;

    s.nodes.forEach(n => {
      yaml += `  - name: "${n.name}"
    address: "${n.address}"
    sni: "${n.sni}"
    transport: "${n.transport}"
    transport_opts:
      ws_path: "${n.transportOpts?.wsPath || '/'}"
      ws_host: "${n.transportOpts?.wsHost || ''}"${n.transportOpts?.socks5Addr ? '\n      socks5_addr: "' + n.transportOpts.socks5Addr + '"' : ''}${n.remoteProxy?.socks5 || n.remoteProxy?.fallback ? `
    remote_proxy:${n.remoteProxy?.socks5 ? '\n      socks5: "' + n.remoteProxy.socks5 + '"' : ''}${n.remoteProxy?.fallback ? '\n      fallback: "' + n.remoteProxy.fallback + '"' : ''}` : ''}
    retry:
      max_attempts: ${n.retry?.maxAttempts || 3}
      base_delay: "${n.retry?.baseDelay || '500ms'}"
      max_delay: "${n.retry?.maxDelay || '5s'}"
      jitter: ${n.retry?.jitter || 0.3}
    pool:
      max_idle: ${n.pool?.maxIdle || 10}
      max_per_key: ${n.pool?.maxPerKey || 5}
      idle_timeout: "${n.pool?.idleTimeout || '120s'}"
      max_lifetime: "${n.pool?.maxLifetime || '10m'}"${n.fingerprint ? '\n    fingerprint: "' + n.fingerprint + '"' : ''}
    active: ${n.active}
`;
    });

    return yaml;
  },

  copy() {
    const yaml = this.generateYAML();
    navigator.clipboard.writeText(yaml).then(() => App.toast('已复制到剪贴板', 'success'));
  },

  async download() {
    const yaml = this.generateYAML();
    try {
      const path = await API.saveConfigFile(yaml);
      if (path) {
        App.toast('已保存到: ' + path, 'success');
        App.log('info', '配置已保存: ' + path);
      }
    } catch (e) {
      // Wails 不可用时回退到浏览器下载
      App.downloadFile('config.yaml', yaml);
      App.toast('配置已下载', 'success');
    }
  },

  refresh() {
    App.renderPage('config');
    App.toast('已刷新', 'info');
  },

  escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }
};
