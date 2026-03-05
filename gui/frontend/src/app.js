/**
 * TLS-Client GUI 桌面版主应用
 */
const App = {
  currentPage: 'dashboard',

  state: {
    engineRunning: false,
    localEngineRunning: false,
    selectedFingerprint: 'chrome-126-win',
    rotationMode: 'fixed',
    tlsVerifyMode: 'sni-skip',
    cadenceMode: 'browsing',
    cadenceJitter: 0.3,
    cadenceMin: '',
    cadenceMax: '',
    cadenceSeq: '',
    cookieEnabled: true,
    cookieClearOnRotation: false,
    followRedirects: true,
    maxRedirects: 10,
    socks5Listen: '127.0.0.1:1080',
    socks5User: '',
    socks5Pass: '',
    httpListen: '127.0.0.1:8080',
    transportFallback: 'ws,h2,raw',
    apiAddress: 'http://127.0.0.1:9090',
    apiToken: '',
    logs: [],
    nodes: [
      {
        name: 'cf-worker-example',
        address: '162.159.19.211:443',
        sni: 'your-worker.workers.dev',
        transport: 'ws',
        fingerprint: '',
        active: true,
        transportOpts: { wsPath: '/', wsHost: '', socks5Addr: '' },
        remoteProxy: { socks5: '', fallback: '' },
        retry: { maxAttempts: 3, baseDelay: '500ms', maxDelay: '5s', jitter: 0.3 },
        pool: { maxIdle: 10, maxPerKey: 5, idleTimeout: '120s', maxLifetime: '10m' },
      }
    ],
    fingerprints: [
      { name:'chrome-120-win', browser:'Chrome', platform:'Windows', version:'120.0', tags:[] },
      { name:'chrome-123-win', browser:'Chrome', platform:'Windows', version:'123.0', tags:[] },
      { name:'chrome-124-win', browser:'Chrome', platform:'Windows', version:'124.0', tags:[] },
      { name:'chrome-124-mac', browser:'Chrome', platform:'macOS', version:'124.0', tags:[] },
      { name:'chrome-125-win', browser:'Chrome', platform:'Windows', version:'125.0', tags:[] },
      { name:'chrome-126-win', browser:'Chrome', platform:'Windows', version:'126.0', tags:['latest','recommended','default'] },
      { name:'chrome-126-mac', browser:'Chrome', platform:'macOS', version:'126.0', tags:['latest'] },
      { name:'chrome-126-android', browser:'Chrome', platform:'Android', version:'126.0', tags:['mobile'] },
      { name:'edge-124-win', browser:'Edge', platform:'Windows', version:'124.0', tags:[] },
      { name:'edge-126-win', browser:'Edge', platform:'Windows', version:'126.0', tags:['latest'] },
      { name:'edge-126-mac', browser:'Edge', platform:'macOS', version:'126.0', tags:['latest'] },
      { name:'firefox-121-win', browser:'Firefox', platform:'Windows', version:'121.0', tags:[] },
      { name:'firefox-124-win', browser:'Firefox', platform:'Windows', version:'124.0', tags:[] },
      { name:'firefox-126-linux', browser:'Firefox', platform:'Linux', version:'126.0', tags:[] },
      { name:'firefox-127-win', browser:'Firefox', platform:'Windows', version:'127.0', tags:['latest'] },
      { name:'firefox-127-mac', browser:'Firefox', platform:'macOS', version:'127.0', tags:['latest'] },
      { name:'safari-17-mac', browser:'Safari', platform:'macOS', version:'17.4.1', tags:[] },
      { name:'safari-17-ios', browser:'Safari', platform:'iOS', version:'17.4.1', tags:['mobile'] },
      { name:'safari-175-mac', browser:'Safari', platform:'macOS', version:'17.5', tags:['latest'] },
      { name:'safari-175-ios', browser:'Safari', platform:'iOS', version:'17.5', tags:['latest','mobile'] },
    ],
    getActiveNode() {
      return this.nodes.find(n => n.active) || this.nodes[0] || null;
    }
  },

  pages: {
    dashboard: DashboardPage,
    nodes: NodesPage,
    fingerprint: FingerprintPage,
    transport: TransportPage,
    cadence: CadencePage,
    inbound: InboundPage,
    logs: LogsPage,
    config: ConfigPage,
    apiconn: ApiConnPage,
  },

  pageTitles: {
    dashboard: '仪表盘',
    nodes: '节点管理',
    fingerprint: '指纹实验室',
    transport: '传输层',
    cadence: '时序控制',
    inbound: '入站代理',
    logs: '日志',
    config: '配置导出',
    apiconn: '连接管理',
  },

  navigate(page) {
    if (!this.pages[page]) return;
    this.currentPage = page;
    document.querySelectorAll('.nav-link').forEach(l => l.classList.toggle('active', l.dataset.page === page));
    const t = document.getElementById('topbar-title');
    if (t) t.textContent = this.pageTitles[page] || page;
    this.renderPage(page);
  },

  renderPage(page) {
    const container = document.getElementById('page-container');
    const pg = this.pages[page || this.currentPage];
    if (container && pg) container.innerHTML = pg.render();
  },

  log(level, message) {
    const time = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    const entry = { time, level, message };
    this.state.logs.push(entry);
    if (this.state.logs.length > 3000) this.state.logs.shift();
    if (this.currentPage === 'logs' && LogsPage.appendLog) LogsPage.appendLog(entry);
  },

  toast(message, type = 'info') {
    const c = document.getElementById('toast-container');
    if (!c) return;
    const t = document.createElement('div');
    t.className = `toast toast-${type}`;
    t.innerHTML = `<span>${message}</span><button class="toast-close" onclick="this.parentElement.remove()">×</button>`;
    c.appendChild(t);
    setTimeout(() => { t.style.animation = 'toast-out .3s ease forwards'; setTimeout(() => t.remove(), 300); }, 4000);
  },

  updateApiIndicator(connected) {
    const ind = document.getElementById('global-api-indicator');
    if (!ind) return;
    const dot = ind.querySelector('.indicator-dot');
    const txt = ind.querySelector('.indicator-text');
    if (dot) { dot.classList.toggle('connected', connected); dot.classList.toggle('disconnected', !connected); }
    if (txt) txt.textContent = connected ? '已连接' : '未连接';
  },

  formatBytes(b) {
    if (b === 0) return '0 B';
    const k = 1024, s = ['B','KB','MB','GB','TB'];
    const i = Math.floor(Math.log(b) / Math.log(k));
    return parseFloat((b / Math.pow(k, i)).toFixed(1)) + ' ' + s[i];
  },

  downloadFile(name, content) {
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = name; a.click();
    URL.revokeObjectURL(url);
  },

  async init() {
    // 侧边栏折叠
    document.getElementById('sidebar-toggle')?.addEventListener('click', () => {
      document.getElementById('sidebar')?.classList.toggle('collapsed');
    });

    // 导航
    document.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', e => { e.preventDefault(); this.navigate(link.dataset.page); });
    });

    // 时钟
    const updateClock = () => {
      const el = document.getElementById('topbar-clock');
      if (el) el.textContent = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    };
    updateClock();
    setInterval(updateClock, 1000);

    // 平台标识
    try {
      const info = await API.getSystemInfo();
      const plat = document.getElementById('topbar-platform');
      if (plat) plat.textContent = `${info.os}/${info.arch}`;
      if (info.os === 'darwin') document.body.classList.add('darwin');
    } catch (e) {
      // 非 Wails 环境下忽略
    }

    // 监听引擎事件
    if (window.runtime) {
      window.runtime.EventsOn('engine:log', (line) => {
        this.log('debug', line.trim());
      });
      window.runtime.EventsOn('engine:stopped', () => {
        this.state.localEngineRunning = false;
        this.toast('本地引擎已停止', 'warning');
        this.log('warn', '本地引擎已停止');
      });
      window.runtime.EventsOn('engine:ready', () => {
        this.state.localEngineRunning = true;
        this.toast('本地引擎已就绪，API 已自动连接', 'success');
        this.log('info', '本地引擎已就绪');
        this.updateApiIndicator(true);
      });
    }

    // 渲染
    this.navigate(this.currentPage);
    this.log('info', 'TLS-Client 桌面版 v3.5 已加载');
    this.log('info', `${this.state.fingerprints.length} 个指纹, ${this.state.nodes.length} 个节点`);
  }
};

document.addEventListener('DOMContentLoaded', () => App.init());
