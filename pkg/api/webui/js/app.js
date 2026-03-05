/**
 * TLS-Client GUI 主应用
 */
const App = {
  currentPage: 'dashboard',

  // ==================== 全局状态 ====================
  state: {
    engineRunning: false,
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

  // ==================== 页面注册 ====================
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
    transport: '传输层设置',
    cadence: '时序控制',
    inbound: '入站代理',
    logs: '日志控制台',
    config: '配置导出',
    apiconn: 'API 连接',
  },

  // ==================== 导航 ====================
  navigate(page) {
    if (!this.pages[page]) return;
    this.currentPage = page;

    document.querySelectorAll('.nav-link').forEach(l => {
      l.classList.toggle('active', l.dataset.page === page);
    });

    const title = document.getElementById('topbar-title');
    if (title) title.textContent = this.pageTitles[page] || page;

    this.renderPage(page);

    // 关闭移动端菜单
    document.getElementById('sidebar')?.classList.remove('mobile-open');
  },

  renderPage(page) {
    const container = document.getElementById('page-container');
    const pg = this.pages[page || this.currentPage];
    if (container && pg) container.innerHTML = pg.render();
  },

  // ==================== 日志 ====================
  log(level, message) {
    const now = new Date();
    const time = now.toLocaleTimeString('zh-CN', { hour12: false });
    const entry = { time, level, message };
    this.state.logs.push(entry);
    if (this.state.logs.length > 2000) this.state.logs.shift();
    if (this.currentPage === 'logs') LogsPage.appendLog(entry);
  },

  // ==================== Toast ====================
  toast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    if (!container) return;
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.innerHTML = `<span>${message}</span><button class="toast-close" onclick="this.parentElement.remove()">×</button>`;
    container.appendChild(toast);
    setTimeout(() => {
      toast.style.animation = 'toast-out .3s ease forwards';
      setTimeout(() => toast.remove(), 300);
    }, 4000);
  },

  // ==================== API 状态指示 ====================
  updateApiIndicator(connected) {
    const ind = document.getElementById('global-api-indicator');
    if (!ind) return;
    const dot = ind.querySelector('.indicator-dot');
    const text = ind.querySelector('.indicator-text');
    if (dot) { dot.classList.toggle('connected', connected); dot.classList.toggle('disconnected', !connected); }
    if (text) text.textContent = connected ? 'API 已连接' : 'API 未连接';
  },

  // ==================== 工具函数 ====================
  formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  },

  downloadFile(filename, content) {
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = filename; a.click();
    URL.revokeObjectURL(url);
  },

  // ==================== 初始化 ====================
  init() {
    // 侧边栏折叠
    document.getElementById('sidebar-toggle')?.addEventListener('click', () => {
      document.getElementById('sidebar')?.classList.toggle('collapsed');
    });

    // 移动端菜单
    document.getElementById('mobile-menu-btn')?.addEventListener('click', () => {
      document.getElementById('sidebar')?.classList.toggle('mobile-open');
    });

    // 导航
    document.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', e => {
        e.preventDefault();
        this.navigate(link.dataset.page);
      });
    });

    // Hash 路由
    const hash = window.location.hash.replace('#', '');
    if (hash && this.pages[hash]) this.currentPage = hash;
    window.addEventListener('hashchange', () => {
      const h = window.location.hash.replace('#', '');
      if (h && this.pages[h]) this.navigate(h);
    });

    // 时钟
    const updateClock = () => {
      const el = document.getElementById('topbar-clock');
      if (el) el.textContent = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    };
    updateClock();
    setInterval(updateClock, 1000);

    // 渲染初始页面
    this.navigate(this.currentPage);
    this.log('info', 'TLS-Client 控制面板 v3.5 已加载');
    this.log('info', `已加载 ${this.state.fingerprints.length} 个指纹, ${this.state.nodes.length} 个节点`);
  }
};

// 启动
document.addEventListener('DOMContentLoaded', () => App.init());
