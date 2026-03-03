// TLS-Client Worker v3.5 (Xlink 兼容版)
// 支持: 直连 / SOCKS5代理出站 / Fallback借力

import { connect } from 'cloudflare:sockets';

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    
    // 健康检查
    if (url.pathname === "/health") {
      return new Response(JSON.stringify({ status: "ok", version: "3.5.0" }), {
        headers: { "Content-Type": "application/json" }
      });
    }
    
    // 检查 WebSocket 升级
    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("Service Online", { status: 200 });
    }
    
    const [client, server] = Object.values(new WebSocketPair());
    server.accept();
    
    ctx.waitUntil(handleSession(server));
    return new Response(null, { status: 101, webSocket: client });
  }
};

async function handleSession(ws) {
  const { readable, writable } = wsToStreams(ws);
  const reader = readable.getReader();
  
  try {
    // 读取第一帧 (Xlink 协议头)
    const { value: chunk, done } = await reader.read();
    if (done || !chunk) return;
    
    // ===== 解析 Xlink 协议头 =====
    const header = parseXlinkHeader(chunk);
    if (!header) return;
    
    const { host, port, socks5, fallback, payload } = header;
    
    console.log(`[Tunnel] Target: ${host}:${port}, S5: ${socks5 || 'none'}, FB: ${fallback || 'none'}`);
    
    // ===== 连接策略工厂 =====
    const factories = [];
    
    // 策略1: SOCKS5 代理
    if (socks5) {
      factories.push(() => connectViaSocks5(socks5, host, port));
    }
    
    // 策略2: 直连
    factories.push(() => tryConnect({ hostname: host, port }));
    
    // 策略3: Fallback
    if (fallback) {
      factories.push(() => {
        const [fbHost, fbPort] = parseHostPort(fallback, 443);
        return tryConnect({ hostname: fbHost, port: fbPort });
      });
    }
    
    // 顺序尝试连接
    let socket = null;
    for (const factory of factories) {
      try {
        socket = await factory();
        if (socket) break;
      } catch (e) {
        console.log(`[Tunnel] Connect attempt failed: ${e.message}`);
      }
    }
    
    if (!socket) {
      console.log("[Tunnel] All connection attempts failed");
      return;
    }
    
    // 发送初始数据 (Early Data)
    if (payload && payload.length > 0) {
      const writer = socket.writable.getWriter();
      await writer.write(payload);
      writer.releaseLock();
    }
    
    reader.releaseLock();
    
    // 双向管道 (零拷贝)
    await Promise.race([
      readable.pipeTo(socket.writable).catch(() => {}),
      socket.readable.pipeTo(writable).catch(() => {})
    ]);
    
  } catch (e) {
    console.error("[Tunnel] Error:", e);
  }
}

// 解析 Xlink 协议头
function parseXlinkHeader(chunk) {
  if (chunk.length < 4) return null;
  
  let cursor = 0;
  const view = new DataView(chunk.buffer, chunk.byteOffset, chunk.byteLength);
  
  // Host
  const hostLen = chunk[cursor]; cursor += 1;
  if (chunk.length < cursor + hostLen) return null;
  const host = new TextDecoder().decode(chunk.slice(cursor, cursor + hostLen));
  cursor += hostLen;
  
  // Port
  if (chunk.length < cursor + 2) return null;
  const port = view.getUint16(cursor);
  cursor += 2;
  
  // SOCKS5
  if (chunk.length < cursor + 1) return null;
  const s5Len = chunk[cursor]; cursor += 1;
  let socks5 = "";
  if (s5Len > 0) {
    if (chunk.length < cursor + s5Len) return null;
    socks5 = new TextDecoder().decode(chunk.slice(cursor, cursor + s5Len));
    cursor += s5Len;
  }
  
  // Fallback
  if (chunk.length < cursor + 1) return null;
  const fbLen = chunk[cursor]; cursor += 1;
  let fallback = "";
  if (fbLen > 0) {
    if (chunk.length < cursor + fbLen) return null;
    fallback = new TextDecoder().decode(chunk.slice(cursor, cursor + fbLen));
    cursor += fbLen;
  }
  
  // Payload
  const payload = chunk.slice(cursor);
  
  return { host, port, socks5, fallback, payload };
}

// SOCKS5 代理连接 (完整实现)
async function connectViaSocks5(s5addr, targetHost, targetPort) {
  // 解析 SOCKS5 地址: user:pass@ip:port
  let user = null, pass = null, proxyHost = s5addr, proxyPort = 1080;
  
  if (s5addr.includes('@')) {
    const atIdx = s5addr.lastIndexOf('@');
    const authPart = s5addr.slice(0, atIdx);
    proxyHost = s5addr.slice(atIdx + 1);
    const colonIdx = authPart.indexOf(':');
    if (colonIdx !== -1) {
      user = authPart.slice(0, colonIdx);
      pass = authPart.slice(colonIdx + 1);
    } else {
      user = authPart;
    }
  }
  
  const [ph, pp] = parseHostPort(proxyHost, 1080);
  proxyHost = ph;
  proxyPort = pp;
  
  // 连接 SOCKS5 代理
  const socket = connect({ hostname: proxyHost, port: proxyPort });
  await socket.opened;
  
  const writer = socket.writable.getWriter();
  const reader = socket.readable.getReader();
  
  // SOCKS5 握手
  await writer.write(user ? new Uint8Array([5, 2, 0, 2]) : new Uint8Array([5, 1, 0]));
  
  let resp = await readBytes(reader, 2);
  if (!resp || resp[0] !== 5) throw new Error("Invalid SOCKS5 response");
  
  // 认证
  if (resp[1] === 2) {
    if (!user) throw new Error("SOCKS5 auth required");
    const userBytes = new TextEncoder().encode(user);
    const passBytes = new TextEncoder().encode(pass || "");
    const authReq = new Uint8Array(3 + userBytes.length + passBytes.length);
    authReq[0] = 1;
    authReq[1] = userBytes.length;
    authReq.set(userBytes, 2);
    authReq[2 + userBytes.length] = passBytes.length;
    authReq.set(passBytes, 3 + userBytes.length);
    await writer.write(authReq);
    
    const authResp = await readBytes(reader, 2);
    if (!authResp || authResp[1] !== 0) throw new Error("SOCKS5 auth failed");
  } else if (resp[1] !== 0) {
    throw new Error("SOCKS5 method not supported");
  }
  
  // CONNECT 请求
  const domainBytes = new TextEncoder().encode(targetHost);
  const req = new Uint8Array(7 + domainBytes.length);
  req[0] = 5; req[1] = 1; req[2] = 0; req[3] = 3;
  req[4] = domainBytes.length;
  req.set(domainBytes, 5);
  req[5 + domainBytes.length] = (targetPort >> 8) & 0xFF;
  req[6 + domainBytes.length] = targetPort & 0xFF;
  await writer.write(req);
  
  // 读取响应
  resp = await readBytes(reader, 5);
  if (!resp || resp[1] !== 0) throw new Error("SOCKS5 connect failed");
  
  // 跳过绑定地址
  const atyp = resp[3];
  if (atyp === 1) await readBytes(reader, 4 + 2 - 1);
  else if (atyp === 3) {
    const dlen = resp[4];
    await readBytes(reader, dlen + 2);
  } else if (atyp === 4) await readBytes(reader, 16 + 2 - 1);
  
  writer.releaseLock();
  reader.releaseLock();
  
  return socket;
}

// 辅助函数
function wsToStreams(ws) {
  const writable = new WritableStream({
    write(chunk) { if (ws.readyState === 1) ws.send(chunk); },
    close() { if (ws.readyState === 1) ws.close(1000); }
  });
  const readable = new ReadableStream({
    start(controller) {
      ws.addEventListener("message", e => {
        if (e.data instanceof ArrayBuffer) controller.enqueue(new Uint8Array(e.data));
      });
      ws.addEventListener("close", () => { try { controller.close(); } catch {} });
      ws.addEventListener("error", e => { try { controller.error(e); } catch {} });
    }
  });
  return { readable, writable };
}

function parseHostPort(s, defaultPort) {
  if (!s) return ["", defaultPort];
  const m = s.match(/^\[(.+)\]:(\d+)$/);
  if (m) return [m[1], parseInt(m[2])];
  const parts = s.split(':');
  return parts.length === 2 ? [parts[0], parseInt(parts[1])] : [s, defaultPort];
}

async function tryConnect(options) {
  const socket = connect(options);
  await socket.opened;
  return socket;
}

async function readBytes(reader, minBytes) {
  let buffer = new Uint8Array(0);
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    const { value, done } = await reader.read();
    if (done) break;
    if (value?.length) {
      const newBuf = new Uint8Array(buffer.length + value.length);
      newBuf.set(buffer, 0);
      newBuf.set(value, buffer.length);
      buffer = newBuf;
      if (buffer.length >= minBytes) return buffer;
    }
  }
  return buffer.length >= minBytes ? buffer : null;
}
