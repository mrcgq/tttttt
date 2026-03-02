import { connect } from 'cloudflare:sockets';

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

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

    const upgradeHeader = request.headers.get("Upgrade");
    if (upgradeHeader === "websocket") {
      return await handleWebSocket(request);
    }

    if (request.method === "POST") {
      const pathname = url.pathname;
      if (pathname === "/" || pathname === "/tunnel" || pathname.startsWith("/tunnel")) {
        return await handlePostTunnel(request, ctx);
      }
    }

    return new Response("TLS-Client Worker Ready", {
      status: 200,
      headers: { "Content-Type": "text/plain" }
    });
  },
};

async function handlePostTunnel(request, ctx) {
  let reader;
  try {
    reader = request.body.getReader();
  } catch (e) {
    return new Response("Invalid request body", { status: 400 });
  }

  let buffer = new Uint8Array(0);
  let targetStr = null;
  let remainingData = null;

  while (targetStr === null) {
    const { done, value } = await reader.read();
    if (done) {
      return new Response("No target address received", { status: 400 });
    }

    const newBuffer = new Uint8Array(buffer.length + value.length);
    newBuffer.set(buffer);
    newBuffer.set(value, buffer.length);
    buffer = newBuffer;

    const newlineIdx = findNewline(buffer);
    if (newlineIdx !== -1) {
      targetStr = new TextDecoder().decode(buffer.slice(0, newlineIdx)).trim();
      remainingData = buffer.slice(newlineIdx + 1);
      break;
    }

    if (buffer.length > 2048) {
      return new Response("Target line too long", { status: 400 });
    }
  }

  if (!targetStr) {
    return new Response("Empty target address", { status: 400 });
  }

  const [host, port] = parseTarget(targetStr);
  console.log(`POST Tunnel: connecting to ${host}:${port}`);

  let targetSocket;
  try {
    targetSocket = connect({ hostname: host, port: parseInt(port) });
    await targetSocket.opened;
  } catch (e) {
    console.error("Connect failed:", e);
    return new Response(`Connection failed: ${e.message}`, { status: 502 });
  }

  const { readable, writable } = new TransformStream();
  const responseWriter = writable.getWriter();

  ctx.waitUntil((async () => {
    try {
      const targetWriter = targetSocket.writable.getWriter();

      if (remainingData && remainingData.length > 0) {
        await targetWriter.write(remainingData);
      }

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
      } catch (closeErr) {}
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

        const target = typeof event.data === "string"
          ? event.data
          : new TextDecoder().decode(new Uint8Array(event.data));

        const [host, port] = parseTarget(target);
        console.log(`WS Tunnel: connecting to ${host}:${port}`);

        try {
          targetSocket = connect({ hostname: host, port: parseInt(port) });
          await targetSocket.opened;
          targetWriter = targetSocket.writable.getWriter();

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
      } catch (e) {}
    }
  });

  server.addEventListener("error", (e) => {
    console.error("WS error:", e);
    if (targetSocket) {
      try {
        targetSocket.close();
      } catch (err) {}
    }
  });

  return new Response(null, { status: 101, webSocket: client });
}

function findNewline(arr) {
  for (let i = 0; i < arr.length; i++) {
    if (arr[i] === 10) {
      return i;
    }
  }
  return -1;
}

function parseTarget(target) {
  target = target.trim();

  if (target.startsWith("[")) {
    const bracketEnd = target.indexOf("]");
    if (bracketEnd !== -1) {
      if (target.length > bracketEnd + 1 && target[bracketEnd + 1] === ":") {
        return [target.slice(1, bracketEnd), target.slice(bracketEnd + 2)];
      }
      return [target.slice(1, bracketEnd), "443"];
    }
  }

  const lastColon = target.lastIndexOf(":");
  if (lastColon === -1) {
    return [target, "443"];
  }

  const beforeColon = target.slice(0, lastColon);
  if (beforeColon.includes(":")) {
    return [target, "443"];
  }

  return [target.slice(0, lastColon), target.slice(lastColon + 1)];
}
