// Produces a paste-ready Snippet with the ticket key already inlined.
//
// Hand-editing the key inside a 9 KB file is the most common way to end up
// with "ticket cannot be decrypted": a missing trailing '=', a stray space or
// a key copied from another deployment all decode to a different 32 bytes.
//
// Usage:
//   node make-snippet.mjs <ticket-key> [--site mysite] [--min] [--out file]
//
// Without --out the result is written to stdout.
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);

function take(flag) {
  const index = args.indexOf(flag);
  if (index === -1) {
    return undefined;
  }
  const value = args[index + 1];
  args.splice(index, 2);
  return value;
}

const site = take("--site");
const out = take("--out");
const min = args.includes("--min");
const rawKey = args.find((a) => !a.startsWith("--"));

if (!rawKey) {
  console.error("usage: node make-snippet.mjs <ticket-key> [--site mysite] [--min] [--out file]");
  process.exit(2);
}

function decodeKey(raw) {
  for (const enc of ["base64", "base64url"]) {
    try {
      const buf = Buffer.from(String(raw).trim(), enc);
      if (buf.length === 32) {
        return buf;
      }
    } catch {
      // try the next encoding
    }
  }
  const raw32 = Buffer.from(String(raw), "utf8");
  if (raw32.length === 32) {
    return raw32;
  }
  throw new Error("ticket key must be 32 bytes (base64 or raw)");
}

const key = decodeKey(rawKey);
const canonical = key.toString("base64");
const fingerprint = crypto.createHash("sha256").update(key).digest("hex").slice(0, 12);

const file = min ? "snippet.min.js" : "snippet.js";
let source = fs.readFileSync(path.join(here, file), "utf8");

if (min) {
  source = source.replace(
    'const FK={"":"REPLACE_WITH_32_BYTE_BASE64_KEY"}',
    site ? `const FK={"${site}":"${canonical}"}` : `const FK={"":"${canonical}"}`,
  );
} else {
  const block = site
    ? `const FALLBACK_TICKET_KEYS = {\n  "": "${canonical}",\n  "${site}": "${canonical}",\n};`
    : `const FALLBACK_TICKET_KEYS = {\n  "": "${canonical}",\n};`;
  source = source.replace(
    /const FALLBACK_TICKET_KEYS = \{[\s\S]*?\n\};/,
    block,
  );
}

if (source.includes("REPLACE_WITH_32_BYTE_BASE64_KEY")) {
  throw new Error("failed to inject the ticket key");
}
if (/[^\x00-\x7F]/.test(source)) {
  throw new Error("generated file must stay pure ASCII");
}

if (out) {
  fs.writeFileSync(out, source);
  console.error(`wrote ${out} (${Buffer.byteLength(source)} bytes, fingerprint ${fingerprint})`);
} else {
  process.stdout.write(source);
  console.error(`fingerprint ${fingerprint} (compare with: node check-edge.mjs <edge> <key>)`);
}
