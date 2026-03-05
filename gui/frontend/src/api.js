/**
 * GUI 桌面版 API 层 — 通过 Wails Go 绑定通信
 */
const API = {
  connected: false,
  pollTimer: null,

  async connect(address, token) {
    try {
      const result = await window.go.main.App.ConnectAPI(address, token);
      this.connected = true;
      return result;
    } catch (e) {
      this.connected = false;
      throw e;
    }
  },

  async disconnect() {
    await window.go.main.App.DisconnectAPI();
    this.connected = false;
  },

  async isConnected() {
    this.connected = await window.go.main.App.IsAPIConnected();
    return this.connected;
  },

  async getStatus() {
    return window.go.main.App.GetStatus();
  },

  async getFingerprints() {
    return window.go.main.App.GetFingerprints();
  },

  async getProxies() {
    return window.go.main.App.GetProxies();
  },

  async getTransports() {
    return window.go.main.App.GetTransports();
  },

  async getDialMetrics() {
    return window.go.main.App.GetDialMetrics();
  },

  async getConfig() {
    return window.go.main.App.GetConfig();
  },

  async postConfig(data) {
    return window.go.main.App.PostConfig(data);
  },

  async startEngine() {
    return window.go.main.App.StartEngine();
  },

  async stopEngine() {
    return window.go.main.App.StopEngine();
  },

  async reloadEngine() {
    return window.go.main.App.ReloadEngine();
  },

  // 本地引擎管理
  async startLocalEngine(configPath) {
    return window.go.main.App.StartLocalEngine(configPath);
  },

  async stopLocalEngine() {
    return window.go.main.App.StopLocalEngine();
  },

  async isLocalRunning() {
    return window.go.main.App.IsLocalEngineRunning();
  },

  async getEngineLogLines() {
    return window.go.main.App.GetEngineLogLines();
  },

  // 文件操作
  async saveConfigFile(content) {
    return window.go.main.App.SaveConfigFile(content);
  },

  async openConfigFile() {
    return window.go.main.App.OpenConfigFile();
  },

  async getSystemInfo() {
    return window.go.main.App.GetSystemInfo();
  },

  /**
   * 启动定时轮询
   * 【修复】：移除 connected 前置检查，改为在回调中处理错误
   * 这样即使 connected 标志延迟同步，轮询也能正常工作
   */
  startPolling(callback, interval = 5000) {
    this.stopPolling();
    const poll = async () => {
      try {
        const data = await this.getStatus();
        this.connected = true; // 能成功获取就说明已连接
        callback(data, null);
      } catch (err) {
        callback(null, err);
      }
    };
    poll(); // 立刻执行一次
    this.pollTimer = setInterval(poll, interval);
  },

  stopPolling() {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }
};
