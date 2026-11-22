// Verifies that the deployed edge can decrypt tickets this Cloudreve issues.
//
// It never sends the ticket key anywhere: it asks the edge for a SHA-256
// fingerprint of the key it holds, then computes the same fingerprint locally
// from the storage policy. Equal fingerprints mean the keys match.
//
// Usage:
//   node check-edge.mjs https://upload.asnndi.com <ticket-key>
//   node check-edge.mjs https://upload.asnndi.com <ticket-key> mysite
//
// Requires the edge to run a build that supports the `x-cr-probe` header.
import crypto from "node:crypto";

const [edgeURL, ticketKey, site] = process.argv.slice(2);
if (!edgeURL || !ticketKey) {
  console.error("usage: node check-edge.mjs <edge-url> <ticket-key> [site]");
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

function fingerprint(siteID, key) {
  const hex = crypto.createHash("sha256").update(key).digest("hex").slice(0, 12);
  return (siteID || "default") + ":" + hex;
}

const expected = fingerprint(site, decodeKey(ticketKey));
console.log("policy key fingerprint :", expected);

const url = edgeURL.replace(/\/+$/, "") + "/";
const res = await fetch(url, { headers: { "x-cr-probe": "1" } });
const body = await res.text();

let parsed;
try {
  parsed = JSON.parse(body);
} catch {
  parsed = null;
}

if (!parsed || !parsed.keys) {
  console.error("edge did not answer the probe (it is probably running an older build).");
  console.error("response:", res.status, body.slice(0, 200));
  process.exit(1);
}

console.log("edge key fingerprints  :", JSON.stringify(parsed.keys));

const edgeFingerprint = parsed.keys[site || "default"] || parsed.keys.default;
if (edgeFingerprint === expected) {
  console.log("MATCH - the edge will decrypt tickets issued by this policy");
} else {
  console.error("MISMATCH - update the edge TICKET_KEYS to match the storage policy");
  process.exit(1);
}
