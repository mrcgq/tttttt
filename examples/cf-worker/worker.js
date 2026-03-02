
// examples/cf-worker/worker.js
// TLS-Client Cloudflare Worker - 支持 WebSocket 和 HTTP POST 隧道

import { connect } from 'cloudflare:sockets';

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // Health check
    if (url.pathname === "/health") {
      return new Response(JSON.stringify({
        status: "ok",
        time: Date.now(),
        version: "1.0.0"
      }), {
        status: 200,
        headers: { "Content-Type": "application/json" }
      });
    }

    // WebSocket Upgrade
    const upgradeHeader = request.headers.get("Upgrade");
    if (upgradeHeader === "websocket") {
      return await handleWebSocket(request);
    }

    // HTTP POST Tunnel (支持 H2 模式)
    if (request.method === "POST") {
      const pathname = url.pathname;
      if (pathname === "/" || pathname === "/tunnel" || pathname.startsWith("/tunnel")) {
        return await handlePostTunnel(request);
      }
    }

    // 默认响应
    return new Response("TLS-Client Worker Ready", {
      status: 200,
      headers: { "Content-Type": "text/plain" }
    });
  },
};

// ==================================================================
// HTTP POST 隧道处理
// ==================================================================
async function handlePostTunnel(request) {
  let reader;
  try {
    reader = request.body.getReader();
  } catch (e) {
    return new Response("Invalid request body", { status: 400 });
  }

  // 步骤1: 读取目标地址（第一行，以换行符结束）
  let buffer = new Uint8Array(0);
  let targetStr = null;
  let remainingData = null;

  while (targetStr === null) {
    const { done, value } = await reader.read();
    if (done) {
      return new Response("No target address received", { status: 400 });
    }

    // 拼接数据
    const newBuffer = new Uint8Array(buffer.length + value.length);
    newBuffer.set(buffer);
    newBuffer.set(value, buffer.length);
    buffer = newBuffer;

    // 查找换行符
    const newlineIdx = findNewline(buffer);
    if (newlineIdx !== -1) {
      targetStr = new TextDecoder().decode(buffer.slice(0, newlineIdx)).trim();
      remainingData = buffer.slice(newlineIdx + 1);
      break;
    }

    // 防止缓冲区过大
    if (buffer.length > 2048) {
      return new Response("Target line too long", { status: 400 });
    }
  }

  if (!targetStr) {
    return new Response("Empty target address", { status: 400 });
  }

  // 步骤2: 解析目标地址
  const [host, port] = parseTarget(targetStr);
  console.log(`POST Tunnel: connecting to ${host}:${port}`);

  // 步骤3: 连接目标服务器
  let targetSocket;
  try {
    targetSocket = connect({ hostname: host, port: parseInt(port) });
    await targetSocket.opened;
  } catch (e) {
    console.error("Connect failed:", e);
    return new Response(`Connection failed: ${e.message}`, { status: 502 });
  }

  // 步骤4: 创建响应流
  const { readable, writable } = new TransformStream();
  const responseWriter = writable.getWriter();

  // 步骤5: 启动双向数据转发

  // 上行: Client -> Target
  ctx.waitUntil((async () => {
    try {
      const targetWriter = targetSocket.writable.getWriter();

      // 先发送剩余数据
      if (remainingData && remainingData.length > 0) {
        await targetWriter.write(remainingData);
      }

      // 继续转发请求体
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        await targetWriter.write(value);
      }

      await targetWriter.close();
    } catch (e) {
      console.error("Uplink error:", e);
    }
  })());

  // 下行: Target -> Client
  ctx.waitUntil((async () => {
    try {
      const targetReader = targetSocket.readable.getReader();
      while (true) {
        const { done, value } = await targetReader.read();
        if (done) break;
        await responseWriter.write(value);
      }
      await responseWriter.close();
    } catch (e) {
      console.error("Downlink error:", e);
      try {
        await responseWriter.close();
      } catch (closeErr) {
        // 忽略关闭错误
      }
    }
  })());

  return new Response(readable, {
    status: 200,
    headers: {
      "Content-Type": "application/octet-stream",
      "Cache-Control": "no-store",
      "X-Accel-Buffering": "no"
    }
  });
}

// ==================================================================
// WebSocket 隧道处理
// ==================================================================
async function handleWebSocket(request) {
  const [client, server] = Object.values(new WebSocketPair());
  server.accept();

  let targetSocket = null;
  let targetWriter = null;
  let firstMessage = true;

  server.addEventListener("message", async (event) => {
    try {
      if (firstMessage) {
        firstMessage = false;

        // 第一条消息是目标地址
        const target = typeof event.data === "string"
          ? event.data
          : new TextDecoder().decode(new Uint8Array(event.data));

        const [host, port] = parseTarget(target);
        console.log(`WS Tunnel: connecting to ${host}:${port}`);

        try {
          targetSocket = connect({ hostname: host, port: parseInt(port) });
          await targetSocket.opened;
          targetWriter = targetSocket.writable.getWriter();

          // 启动下行转发
          const targetReader = targetSocket.readable.getReader();
          (async () => {
            try {
              while (true) {
                const { done, value } = await targetReader.read();
                if (done) break;
                if (server.readyState === WebSocket.OPEN) {
                  server.send(value);
                } else {
                  break;
                }
              }
            } catch (e) {
              console.error("WS downlink error:", e);
            }
            server.close();
          })();

        } catch (e) {
          console.error("WS connect failed:", e);
          server.close(1011, `Connect failed: ${e.message}`);
          return;
        }

        return;
      }

      // 后续消息转发到目标
      if (targetWriter) {
        const data = event.data instanceof ArrayBuffer
          ? new Uint8Array(event.data)
          : new TextEncoder().encode(event.data);
        await targetWriter.write(data);
      }

    } catch (e) {
      console.error("WS message handler error:", e);
      server.close(1011, "Internal error");
    }
  });

  server.addEventListener("close", () => {
    if (targetSocket) {
      try {
        targetSocket.close();
      } catch (e) {
        // 忽略关闭错误
      }
    }
  });

  server.addEventListener("error", (e) => {
    console.error("WS error:", e);
    if (targetSocket) {
      try {
        targetSocket.close();
      } catch (err) {
        // 忽略
      }
    }
  });

  return new Response(null, { status: 101, webSocket: client });
}

// ==================================================================
// 工具函数
// ==================================================================

function findNewline(arr) {
  for (let i = 0; i < arr.length; i++) {
    if (arr[i] === 10) { // \n
      return i;
    }
  }
  return -1;
}

function parseTarget(target) {
  target = target.trim();

  // IPv6 with port: [::1]:443
  if (target.startsWith("[")) {
    const bracketEnd = target.indexOf("]");
    if (bracketEnd !== -1) {
      if (target.length > bracketEnd + 1 && target[bracketEnd + 1] === ":") {
        return [target.slice(1, bracketEnd), target.slice(bracketEnd + 2)];
      }
      return [target.slice(1, bracketEnd), "443"];
    }
  }

  // hostname:port or IPv4:port
  const lastColon = target.lastIndexOf(":");
  if (lastColon === -1) {
    return [target, "443"];
  }

  // 检查是否是 IPv6 无端口
  const beforeColon = target.slice(0, lastColon);
  if (beforeColon.includes(":")) {
    // 可能是 IPv6 地址
    return [target, "443"];
  }

  return [target.slice(0, lastColon), target.slice(lastColon + 1)];
}
