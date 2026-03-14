/**
 * TLS-Client GUI 桌面版主应用
 * 版本: v4.4 - 字段名严格对齐 types.go (含 json tag 修复)
 */
const App = {
  currentPage: 'dashboard',
  state: {
    engineRunning: false,
    localEngineRunning: false,
    selectedFingerprint: 'chrome-126-win',
    rotationMode: 'fixed',
    rotationInterval: '',
    rotationWeights: [],
    tlsVerifyMode: 'sni-skip',
    tlsCertPin: '',
    tlsCustomCA: '',
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
    // ProxyIP 池 — 字段对齐 ProxyIPOptions
    proxyIPEnabled: false,
    proxyIPMode: 'round-robin',
    proxyIPOptions: {
      checkPeriod: '30s',
      timeout: '10s',
      maxFails: 3,
    },
    proxyIPEntries: [],
    // Metrics
    metricsEnabled: false,
    metricsEndpoint: '',
    logs: [],
    nodes: [],
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
    setTimeout(() => {
      t.style.animation = 'toast-out .3s ease forwards';
      setTimeout(() => t.remove(), 300);
    }, 4000);
  },

  updateApiIndicator(connected) {
    const ind = document.getElementById('global-api-indicator');
    if (!ind) return;
    const dot = ind.querySelector('.indicator-dot');
    const txt = ind.querySelector('.indicator-text');
    if (dot) {
      dot.classList.toggle('connected', connected);
      dot.classList.toggle('disconnected', !connected);
    }
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
    a.href = url;
    a.download = name;
    a.click();
    URL.revokeObjectURL(url);
  },

  // ================================================================
  // 从核心引擎获取配置并同步到前端状态
  // ================================================================
  async fetchAndApplyConfig() {
    if (!API.connected) return;

    try {
      const fullConfig = await API.getConfig();
      if (!fullConfig) return;

      // 同步节点列表
      if (fullConfig.nodes && Array.isArray(fullConfig.nodes)) {
        this.state.nodes = fullConfig.nodes.map(n => ({
          name: n.name || '',
          address: n.address || '',
          sni: n.sni || '',
          transport: n.transport || 'ws',
          fingerprint: n.fingerprint || '',
          active: n.active || false,
          transportOpts: {
            wsPath: n.transport_opts?.ws_path || '/',
            wsHost: n.transport_opts?.ws_host || '',
            h2Path: n.transport_opts?.h2_path || '',
            socks5Addr: n.transport_opts?.socks5_addr || '',
            socks5Username: n.transport_opts?.socks5_username || '',
            socks5Password: n.transport_opts?.socks5_password || '',
            wsHeaders: n.transport_opts?.ws_headers || {},
          },
          remoteProxy: {
            socks5: n.remote_proxy?.socks5 || '',
            fallback: n.remote_proxy?.fallback || '',
          },
          retry: {
            maxAttempts: n.retry?.max_attempts || 3,
            baseDelay: n.retry?.base_delay || '500ms',
            maxDelay: n.retry?.max_delay || '5s',
            jitter: n.retry?.jitter || 0.3,
          },
          pool: {
            maxIdle: n.pool?.max_idle || 10,
            maxPerKey: n.pool?.max_per_key || 5,
            idleTimeout: n.pool?.idle_timeout || '120s',
            maxLifetime: n.pool?.max_lifetime || '10m',
          },
          transportFallback: n.transport_fallback || [],
        }));
        console.log('App.state.nodes updated:', this.state.nodes);
      }

      // 同步入站配置
      if (fullConfig.inbound) {
        if (fullConfig.inbound.socks5) {
          this.state.socks5Listen = fullConfig.inbound.socks5.listen || '127.0.0.1:1080';
          this.state.socks5User = fullConfig.inbound.socks5.username || '';
          this.state.socks5Pass = fullConfig.inbound.socks5.password || '';
        }
        if (fullConfig.inbound.http) {
          this.state.httpListen = fullConfig.inbound.http.listen || '127.0.0.1:8080';
        }
      }

      // 同步指纹配置
      if (fullConfig.fingerprint?.rotation) {
        this.state.rotationMode = fullConfig.fingerprint.rotation.mode || 'fixed';
        this.state.selectedFingerprint = fullConfig.fingerprint.rotation.profile || 'chrome-126-win';
        this.state.rotationInterval = fullConfig.fingerprint.rotation.interval || '';
        if (Array.isArray(fullConfig.fingerprint.rotation.weights)) {
          this.state.rotationWeights = fullConfig.fingerprint.rotation.weights;
        }
      }

      // 同步 TLS 配置
      if (fullConfig.tls) {
        this.state.tlsVerifyMode = fullConfig.tls.verify_mode || 'sni-skip';
        if (fullConfig.tls.verify_opts) {
          this.state.tlsCertPin = fullConfig.tls.verify_opts.cert_pin || '';
          this.state.tlsCustomCA = fullConfig.tls.verify_opts.custom_ca || '';
        }
      }

      // 同步客户端行为配置
      if (fullConfig.client_behavior) {
        if (fullConfig.client_behavior.cadence) {
          const rawMode = fullConfig.client_behavior.cadence.mode || 'none';
          // 后端 "custom" → 前端 "sequence"
          this.state.cadenceMode = (rawMode === 'custom') ? 'sequence' : rawMode;
          this.state.cadenceJitter = fullConfig.client_behavior.cadence.jitter || 0.3;
          this.state.cadenceMin = fullConfig.client_behavior.cadence.min_delay || '';
          this.state.cadenceMax = fullConfig.client_behavior.cadence.max_delay || '';
          if (fullConfig.client_behavior.cadence.sequence && Array.isArray(fullConfig.client_behavior.cadence.sequence)) {
            this.state.cadenceSeq = fullConfig.client_behavior.cadence.sequence.join(',');
          }
        }
        if (fullConfig.client_behavior.cookies) {
          this.state.cookieEnabled = fullConfig.client_behavior.cookies.enabled || false;
          this.state.cookieClearOnRotation = fullConfig.client_behavior.cookies.clear_on_rotation || false;
        }
        this.state.followRedirects = fullConfig.client_behavior.follow_redirects !== false;
        this.state.maxRedirects = fullConfig.client_behavior.max_redirects || 10;
      }

      // 同步 Global Metrics
      if (fullConfig.global?.metrics) {
        this.state.metricsEnabled = fullConfig.global.metrics.enabled || false;
        this.state.metricsEndpoint = fullConfig.global.metrics.endpoint || '';
      }

      // 同步 ProxyIP 配置 — 字段对齐 types.go
      if (fullConfig.proxy_ips) {
        this.state.proxyIPEnabled = fullConfig.proxy_ips.enabled || false;
        this.state.proxyIPMode = fullConfig.proxy_ips.mode || 'round-robin';
        if (fullConfig.proxy_ips.options) {
          this.state.proxyIPOptions = {
            checkPeriod: fullConfig.proxy_ips.options.check_period || '30s',
            timeout: fullConfig.proxy_ips.options.timeout || '10s',
            maxFails: fullConfig.proxy_ips.options.max_fails || 3,
          };
        }
        if (fullConfig.proxy_ips.entries && Array.isArray(fullConfig.proxy_ips.entries)) {
          this.state.proxyIPEntries = fullConfig.proxy_ips.entries.map(e => ({
            address: e.address || '',
            sni: e.sni || '',
            weight: e.weight || 1,
            region: e.region || '',
            provider: e.provider || '',
            enabled: e.enabled !== false,
          }));
        }
      }

      // 同步当前活跃配置
      if (fullConfig.current_profile) {
        this.state.selectedFingerprint = fullConfig.current_profile;
      }
      if (fullConfig.current_node) {
        this.state.nodes.forEach(n => {
          n.active = (n.name === fullConfig.current_node);
        });
      }

      this.log('debug', '配置已从核心引擎同步');

    } catch (e) {
      this.log('error', '同步配置失败: ' + e);
    }
  },

  // ================================================================
  // 构建完整配置对象 — 字段对齐 types.go (含 json tag)
  // ================================================================
  buildFullConfig() {
    const s = this.state;

    // 解析 cadenceSeq 为数组
    let sequenceArray = [];
    if (s.cadenceSeq && s.cadenceSeq.trim()) {
      sequenceArray = s.cadenceSeq.split(',').map(v => v.trim()).filter(v => v);
    }

    // cadenceMode 映射: 前端 "sequence" → 后端 "custom"
    let backendCadenceMode = s.cadenceMode;
    if (backendCadenceMode === 'sequence') {
      backendCadenceMode = 'custom';
    }

    // weights: types.go 里是 []int，直接传数组
    const weightsArray = Array.isArray(s.rotationWeights) ? s.rotationWeights : [];

    return {
      global: {
        log_level: "debug",
        log_output: "stderr",
        metrics: {
          enabled: s.metricsEnabled,
          endpoint: s.metricsEndpoint,
        },
      },
      inbound: {
        socks5: {
          listen: s.socks5Listen,
          username: s.socks5User,
          password: s.socks5Pass,
        },
        http: {
          listen: s.httpListen,
        },
      },
      fingerprint: {
        rotation: {
          mode: s.rotationMode,
          profile: s.selectedFingerprint,
          profiles: s.fingerprints.map(f => f.name),
          interval: s.rotationInterval || '',
          weights: weightsArray,
        },
      },
      tls: {
        verify_mode: s.tlsVerifyMode,
        verify_opts: {
          cert_pin: s.tlsCertPin || '',
          custom_ca: s.tlsCustomCA || '',
        },
      },
      client_behavior: {
        cadence: {
          mode: backendCadenceMode,
          jitter: s.cadenceJitter,
          min_delay: s.cadenceMin,
          max_delay: s.cadenceMax,
          sequence: sequenceArray,
        },
        cookies: {
          enabled: s.cookieEnabled,
          clear_on_rotation: s.cookieClearOnRotation,
        },
        follow_redirects: s.followRedirects,
        max_redirects: s.maxRedirects,
      },
      api: {
        enabled: true,
        listen: "127.0.0.1:9090",
      },
      health: {
        enabled: false,
      },
      proxy_ips: {
        enabled: s.proxyIPEnabled,
        mode: s.proxyIPMode,
        options: {
          check_period: s.proxyIPOptions?.checkPeriod || '30s',
          timeout: s.proxyIPOptions?.timeout || '10s',
          max_fails: s.proxyIPOptions?.maxFails || 3,
        },
        entries: (s.proxyIPEntries || []).map(e => ({
          address: e.address,
          sni: e.sni || '',
          weight: e.weight || 1,
          region: e.region || '',
          provider: e.provider || '',
          enabled: e.enabled !== false,
        })),
      },
      nodes: s.nodes.map(n => ({
        name: n.name,
        address: n.address,
        sni: n.sni,
        transport: n.transport,
        fingerprint: n.fingerprint || '',
        active: n.active,
        transport_opts: {
          ws_path: n.transportOpts?.wsPath || '/',
          ws_host: n.transportOpts?.wsHost || '',
          ws_headers: n.transportOpts?.wsHeaders || {},
          h2_path: n.transportOpts?.h2Path || '',
          socks5_addr: n.transportOpts?.socks5Addr || '',
          socks5_username: n.transportOpts?.socks5Username || '',
          socks5_password: n.transportOpts?.socks5Password || '',
        },
        transport_fallback: n.transportFallback || [],
        remote_proxy: {
          socks5: n.remoteProxy?.socks5 || '',
          fallback: n.remoteProxy?.fallback || '',
        },
        retry: {
          max_attempts: n.retry?.maxAttempts || 3,
          base_delay: n.retry?.baseDelay || '500ms',
          max_delay: n.retry?.maxDelay || '5s',
          jitter: n.retry?.jitter || 0.3,
        },
        pool: {
          max_idle: n.pool?.maxIdle || 10,
          max_per_key: n.pool?.maxPerKey || 5,
          idle_timeout: n.pool?.idleTimeout || '120s',
          max_lifetime: n.pool?.maxLifetime || '10m',
        },
      })),
      current_profile: s.selectedFingerprint,
    };
  },

  // ================================================================
  // 发送配置到核心引擎并触发重载
  // ================================================================
  async pushConfigToEngine() {
    if (!API.connected) {
      this.toast('请先连接到引擎 API', 'warning');
      return false;
    }

    try {
      const fullConfig = this.buildFullConfig();
      this.log('debug', '发送配置到引擎: ' + JSON.stringify(fullConfig).substring(0, 200) + '...');

      const result = await API.postConfig(fullConfig);

      if (result && result.success) {
        this.toast('配置已同步到引擎并触发重载', 'success');
        this.log('info', '配置已推送到核心引擎');
        return true;
      } else {
        this.toast('配置同步失败: ' + (result?.message || '未知错误'), 'error');
        this.log('error', '配置同步失败: ' + (result?.message || '未知错误'));
        return false;
      }
    } catch (e) {
      this.toast('配置同步失败: ' + e, 'error');
      this.log('error', '推送配置失败: ' + e);
      return false;
    }
  },

  // ================================================================
  // 初始化应用 (gui/frontend/src/app.js)
  // ================================================================
  async init() {
    document.getElementById('sidebar-toggle')?.addEventListener('click', () => {
      document.getElementById('sidebar')?.classList.toggle('collapsed');
    });

    document.querySelectorAll('.nav-link').forEach(link => {
      link.addEventListener('click', e => {
        e.preventDefault();
        this.navigate(link.dataset.page);
      });
    });

    const updateClock = () => {
      const el = document.getElementById('topbar-clock');
      if (el) el.textContent = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    };
    updateClock();
    setInterval(updateClock, 1000);

    try {
      const info = await API.getSystemInfo();
      const plat = document.getElementById('topbar-platform');
      if (plat) plat.textContent = `${info.os}/${info.arch}`;
      if (info.os === 'darwin') document.body.classList.add('darwin');
    } catch (e) {
      console.log('Not running in Wails environment');
    }

    if (window.runtime) {
      window.runtime.EventsOn('engine:log', (line) => {
        if (line && line.trim()) {
          this.log('debug', line.trim());
        }
      });

      window.runtime.EventsOn('engine:stopped', () => {
        this.state.localEngineRunning = false;
        this.state.engineRunning = false;
        this.toast('本地引擎已停止', 'warning');
        this.log('warn', '本地引擎已停止');

        API.connected = false;
        API.stopPolling();
        this.updateApiIndicator(false);

        const el = document.getElementById('dash-status');
        if (el) {
          el.textContent = '已停止';
          el.className = 'stat-value red';
        }

        const setZero = (id, v) => {
          const e = document.getElementById(id);
          if (e) e.textContent = v;
        };
        setZero('dash-uptime', '—');
        setZero('dash-conns', '0');
        setZero('dash-bytes', '0 B');
        setZero('dash-goroutines', '0');
        setZero('dash-mem', '0 MB');
      });

      window.runtime.EventsOn('engine:ready', async () => {
        this.state.localEngineRunning = true;
        this.state.engineRunning = true;
        this.toast('本地引擎已就绪，数据已连接', 'success');
        this.log('info', '本地引擎已就绪，API 连接成功');

        API.connected = true;
        this.updateApiIndicator(true);

        await this.fetchAndApplyConfig();

        try {
          const data = await API.getStatus();
          if (DashboardPage && DashboardPage.updateFromAPI) {
            DashboardPage.updateFromAPI(data);
          }
        } catch (e) {
          this.log('error', '获取初始状态失败: ' + e);
        }

        API.startPolling((data, err) => {
          if (data) {
            if (DashboardPage && DashboardPage.updateFromAPI) {
              DashboardPage.updateFromAPI(data);
            }
          }
          if (err) {
            this.log('error', 'API 轮询错误: ' + err);
          }
        }, 2000);
      });

      window.runtime.EventsOn('engine:timeout', () => {
        this.toast('引擎启动超时，API 未响应', 'error');
        this.log('error', '引擎启动超时（15秒内未检测到 API）');
        const el = document.getElementById('dash-status');
        if (el) {
          el.textContent = '超时';
          el.className = 'stat-value red';
        }
      });
    }

    this.navigate(this.currentPage);
    this.log('info', 'TLS-Client 桌面版 v4.4 已加载');
    this.log('info', `${this.state.fingerprints.length} 个指纹, ${this.state.nodes.length} 个节点`);

    // ================================================================
    // 🚀 终极体验优化：实现“零点击”全自动启动
    // ================================================================
    setTimeout(async () => {
      if (window.go && window.go.main && window.go.main.App) {
        try {
          // 1. 先尝试静默连接一次 API (万一内核已经由其他方式启动了，或者是在热重载)
          await API.getStatus();
          this.log('info', '检测到 API 已在线，跳过自动启动');
          // 连上了就触发正常的数据同步
          API.connected = true;
          this.updateApiIndicator(true);
          await this.fetchAndApplyConfig();
          API.startPolling((data) => {
            if (data && DashboardPage.updateFromAPI) DashboardPage.updateFromAPI(data);
          }, 2000);
        } catch (e) {
          // 2. 如果连不上 API (报错了)，说明本地内核还没起，触发全自动启动！
          this.log('info', 'API 未响应，正在全自动拉取本地内核...');
          if (DashboardPage && DashboardPage.startLocal) {
            DashboardPage.startLocal(); // 直接复用仪表盘里的“启动本地引擎”逻辑
          }
        }
      }
    }, 500); // 稍微延迟 500ms，等 UI 渲染完毕再执行
  }
};

document.addEventListener('DOMContentLoaded', () => App.init());
