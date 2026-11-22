/**
 * Cloudflare Snippet: 1yunpan edge upload for Cloudreve.
 *
 * The browser posts chunks here. Each request carries a sealed ticket
 * (AES-256-GCM) issued by Cloudreve that contains the 1yunpan session, the
 * target path, the declared size and the worker endpoint. This snippet
 * decrypts it, injects the session server-side and forwards the bytes.
 *
 * The account never reaches the browser and this snippet never stores one, so
 * the same domain can serve any number of Cloudreve sites that share the
 * ticket key. Path and size always come from the ticket, never from headers.
 *
 * Each browser request costs exactly one subrequest, which keeps it inside the
 * 2-subrequest limit of Cloudflare Snippets on the Pro plan.
 *
 * Keep this file dependency-free: only Web-standard APIs that Snippets expose.
 */

// Site id -> ticket key. Each Cloudreve deployment keeps its own key, so one
// public domain can serve many sites without them sharing a secret.
// The empty-string entry is the default used by tickets without a site id.
// Snippets cannot read secrets, so paste the base64 keys here.
// Edit these for a Snippet. A Worker overrides them at request time from the
// TICKET_KEYS secret, so the lookup must stay lazy: `env` is only available
// inside fetch(), while module scope is evaluated once at startup.
const FALLBACK_TICKET_KEYS = {
  "": "REPLACE_WITH_32_BYTE_BASE64_KEY",
  // "mysite": "ANOTHER_32_BYTE_BASE64_KEY",
};

const keyCache = new Map();
let cachedKeysSource = null;

function ticketKeys() {
  const keys = globalThis.__CR_TICKET_KEYS || FALLBACK_TICKET_KEYS;
  // Rotating the secret replaces the object. Drop derived crypto keys so the
  // new value takes effect without waiting for the isolate to recycle.
  if (cachedKeysSource !== keys) {
    cachedKeysSource = keys;
    keyCache.clear();
  }
  return keys;
}

const ALLOWED_HEADERS = [
  "content-type",
  "content-range",
  "x-cr-ticket",
  "x-next-token",
  "x-file-hash",
  "x-file-size",
  "x-file-name",
  "x-upload-uri",
].join(",");

const EXPOSED_HEADERS = "x-cr-session-expired,x-cr-next-token";

const decoder = new TextDecoder();

function corsHeaders(origin) {
  return {
    "access-control-allow-origin": origin || "*",
    "access-control-allow-methods": "POST,OPTIONS",
    "access-control-allow-headers": ALLOWED_HEADERS,
    "access-control-expose-headers": EXPOSED_HEADERS,
    "access-control-max-age": "86400",
    "vary": "Origin",
  };
}

function reply(body, status, origin) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "content-type": "application/json",
      ...corsHeaders(origin),
    },
  });
}

function base64UrlToBytes(value) {
  let text = String(value || "").trim().replace(/-/g, "+").replace(/_/g, "/");
  if (!/^[A-Za-z0-9+/]*={0,2}$/.test(text)) {
    throw new Error("invalid ticket encoding");
  }
  while (text.length % 4 !== 0) {
    text += "=";
  }
  let raw;
  try {
    raw = atob(text);
  } catch {
    throw new Error("invalid ticket encoding");
  }
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) {
    bytes[i] = raw.charCodeAt(i);
  }
  return bytes;
}

function ticketKey(site) {
  const id = site || "";
  const keys = ticketKeys();
  // Fall back to the default entry when a site id is set on the policy but not
  // listed here. The key is what actually protects the ticket; the site id is
  // only a selector, so falling back cannot weaken security and it prevents a
  // one-character config mismatch from breaking every upload.
  const encoded = keys[id] || keys[""];
  if (!encoded) {
    throw new Error("unknown edge site");
  }
  const cacheKey = keys[id] ? id : "";
  let cached = keyCache.get(cacheKey);
  if (!cached) {
    const raw = base64UrlToBytes(encoded.trim());
    if (raw.length !== 32) {
      throw new Error("ticket key must decode to 32 bytes");
    }
    cached = crypto.subtle.importKey("raw", raw, { name: "AES-GCM" }, false, ["decrypt"]);
    keyCache.set(cacheKey, cached);
  }
  return cached;
}

// Short non-reversible fingerprint of the configured key. It lets an operator
// compare "what the edge holds" with "what the policy holds" without ever
// transmitting the key itself. Also returned with decrypt failures.
async function keyFingerprint(site) {
  const id = site || "";
  const keys = ticketKeys();
  const encoded = keys[id] || keys[""];
  if (!encoded) {
    return "none";
  }
  try {
    const digest = await crypto.subtle.digest("SHA-256", base64UrlToBytes(encoded.trim()));
    const bytes = new Uint8Array(digest);
    let hex = "";
    for (let i = 0; i < 6; i++) {
      hex += bytes[i].toString(16).padStart(2, "0");
    }
    return (id ? id : "default") + ":" + hex;
  } catch {
    return "invalid";
  }
}

// Reports one fingerprint per configured site without revealing any key
// material. `curl -H 'x-cr-probe: 1' https://upload.example.com/` turns a
// "ticket cannot be decrypted" report into a one-line comparison.
async function keyFingerprints() {
  const keys = ticketKeys();
  const out = {};
  for (const id of Object.keys(keys)) {
    out[id || "default"] = await keyFingerprint(id);
  }
  return out;
}

async function openTicket(sealed) {
  let site = "";
  let payload = sealed;
  const dot = sealed.indexOf(".");
  if (dot > 0 && dot < 64) {
    site = sealed.slice(0, dot);
    payload = sealed.slice(dot + 1);
  }

  const raw = base64UrlToBytes(payload);
  if (raw.length <= 12) {
    throw new Error("ticket is truncated");
  }

  let plain;
  try {
    plain = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: raw.slice(0, 12) },
      await ticketKey(site),
      raw.slice(12),
    );
  } catch (e) {
    // The detail WebCrypto returns ("operation-specific reason") is useless to
    // an operator; a wrong key and a tampered ticket look identical here.
    if (e && /key/.test(String(e.message))) {
      throw e;
    }
    throw new Error(
      "ticket cannot be decrypted (site " + (site || "default") + ", edge key fingerprint " +
        (await keyFingerprint(site)) + ")",
    );
  }
  const ticket = JSON.parse(decoder.decode(plain));

  if (!ticket.s || !ticket.p || !ticket.z || !ticket.u) {
    throw new Error("ticket is missing required fields");
  }
  if (String(ticket.u).indexOf("https://") !== 0) {
    throw new Error("ticket endpoint must use https");
  }
  if (ticket.e && Date.now() / 1000 > ticket.e) {
    throw new Error("ticket has expired");
  }
  return ticket;
}

function parseRange(value) {
  const match = /^(\d+)-(\d+)$/.exec((value || "").trim());
  if (!match) {
    return null;
  }
  return { start: Number(match[1]), end: Number(match[2]) };
}

// 1yunpan reports an expired login on the final chunk as HTTP 200 with
// {"status":true,"callback":{"status":false,"message":"<invalid token>"}}.
// Treat that as an auth failure so the browser refreshes the ticket instead of
// believing the upload succeeded. The Chinese markers are written as \u escapes
// on purpose: this file must stay pure ASCII so the Cloudflare console cannot
// reject it over text encoding.
function isAuthFailure(parsed) {
  const callback = parsed && parsed.callback;
  if (!callback || callback.status !== false) {
    return false;
  }
  return /[\u4ee4\u724c\u8ba4\u8bc1\u9274\u6743]|unauthori[sz]ed|token|session/i.test(
    String(callback.message || ""),
  );
}

function expired(origin) {
  const res = reply({ status: false, message: "upstream session expired" }, 401, origin);
  res.headers.set("x-cr-session-expired", "1");
  return res;
}

async function forward(request, ticket, range, origin) {
  const size = Number(ticket.z);
  if (range.start < 0 || range.end < range.start || range.end >= size) {
    return reply({ status: false, message: "invalid content range" }, 416, origin);
  }

  const isFinal = range.end + 1 === size;
  const headers = new Headers({
    "content-range": range.start + "-" + range.end,
    "content-type": "application/octet-stream",
  });

  const nextToken = request.headers.get("x-next-token");
  if (nextToken) {
    headers.set("x-next-token", nextToken);
  }

  if (isFinal) {
    const name = ticket.p.split("/").pop() || ticket.p;
    headers.set("x-upload-uri", encodeURIComponent("/" + ticket.p.replace(/^\/+/, "")));
    headers.set("x-phpsessid", ticket.s);
    headers.set("x-file-hash", "0");
    headers.set("x-file-size", String(size));
    headers.set("x-file-name", encodeURIComponent(name));
  }

  const upstream = await fetch(ticket.u.replace(/\/+$/, "") + "/v1/upload", {
    method: "POST",
    headers,
    body: request.body,
    redirect: "manual",
  });

  if (upstream.status === 401 || upstream.status === 403) {
    return expired(origin);
  }

  const text = await upstream.text();
  if (upstream.status < 200 || upstream.status >= 300) {
    return reply(
      { status: false, message: text.slice(0, 512) || "upstream status " + upstream.status },
      upstream.status,
      origin,
    );
  }

  let parsed = {};
  try {
    parsed = JSON.parse(text);
  } catch {
    parsed = {};
  }

  if (isFinal && isAuthFailure(parsed)) {
    return expired(origin);
  }
  if (isFinal && parsed.callback && parsed.callback.status === false) {
    return reply(
      { status: false, message: String(parsed.callback.message || "upstream rejected the upload") },
      502,
      origin,
    );
  }

  const res = reply(
    { status: parsed.status !== false, message: parsed.message, next: parsed.next || "" },
    200,
    origin,
  );
  if (parsed.next) {
    res.headers.set("x-cr-next-token", parsed.next);
  }
  return res;
}

const __handler = {
  async fetch(request) {
    const origin = request.headers.get("origin") || "";

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders(origin) });
    }
    if (request.headers.get("x-cr-probe")) {
      return reply({ status: true, keys: await keyFingerprints() }, 200, origin);
    }
    if (request.method === "GET" || request.method === "HEAD") {
      return new Response("ok", { status: 200, headers: corsHeaders(origin) });
    }
    if (request.method !== "POST") {
      return reply({ status: false, message: "method not allowed" }, 405, origin);
    }

    const sealed = request.headers.get("x-cr-ticket");
    if (!sealed) {
      return reply({ status: false, message: "missing ticket" }, 400, origin);
    }

    const range = parseRange(request.headers.get("content-range"));
    if (!range) {
      return reply({ status: false, message: "missing content-range" }, 400, origin);
    }

    let ticket;
    try {
      ticket = await openTicket(sealed);
    } catch (e) {
      return reply({ status: false, message: e.message || "invalid ticket" }, 403, origin);
    }

    return forward(request, ticket, range, origin);
  },
};

// ---------------------------------------------------------------------------
// Worker entrypoint (generated by build-worker.mjs - do not edit by hand).
//
// Workers can read secrets, so the ticket keys do not have to live in source:
//
//   wrangler secret put TICKET_KEYS
//   {"":"<base64 32-byte key>","mysite":"<another key>"}
//
// The deployment also still works with the hardcoded TICKET_KEYS above.
export default {
  async fetch(request, env, ctx) {
    if (env && env.TICKET_KEYS) {
      try {
        globalThis.__CR_TICKET_KEYS = JSON.parse(env.TICKET_KEYS);
      } catch {
        return new Response(
          JSON.stringify({ status: false, message: "TICKET_KEYS is not valid JSON" }),
          { status: 500, headers: { "content-type": "application/json" } },
        );
      }
    }
    return __handler.fetch(request, env, ctx);
  },
};
