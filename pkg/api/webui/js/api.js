/**
 * TLS-Client API 通信层
 */
const API = {
  address: 'http://127.0.0.1:9090',
  token: '',
  connected: false,
  pollTimer: null,

  setAddress(addr) {
    this.address = addr.replace(/\/+$/, '');
  },

  setToken(tok) {
    this.token = tok || '';
  },

  headers() {
    const h = { 'Content-Type': 'application/json' };
    if (this.token) h['Authorization'] = 'Bearer ' + this.token;
    return h;
  },

  async request(endpoint, method = 'GET', body = null) {
    const opts = { method, headers: this.headers() };
    if (body) opts.body = JSON.stringify(body);
    const resp = await fetch(this.address + endpoint, opts);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
    const text = await resp.text();
    try { return JSON.parse(text); } catch { return text; }
  },

  async getStatus() { return this.request('/api/status'); },
  async getFingerprints() { return this.request('/api/fingerprints'); },
  async getProxies() { return this.request('/api/proxies'); },
  async getTransports() { return this.request('/api/transports'); },
  async getDialMetrics() { return this.request('/api/dial-metrics'); },
  async getConfig() { return this.request('/api/config'); },
  async postConfig(data) { return this.request('/api/config', 'POST', data); },
  async postStart() { return this.request('/api/start', 'POST'); },
  async postStop() { return this.request('/api/stop', 'POST'); },
  async postReload() { return this.request('/api/reload', 'POST'); },

  async testConnection() {
    try {
      await this.getStatus();
      this.connected = true;
      return true;
    } catch {
      this.connected = false;
      return false;
    }
  },

  startPolling(callback, interval = 5000) {
    this.stopPolling();
    const poll = async () => {
      if (!this.connected) return;
      try {
        const data = await this.getStatus();
        callback(data, null);
      } catch (err) {
        callback(null, err);
      }
    };
    poll();
    this.pollTimer = setInterval(poll, interval);
  },

  stopPolling() {
    if (this.pollTimer) { clearInterval(this.pollTimer); this.pollTimer = null; }
  }
};
