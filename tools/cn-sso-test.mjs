import http from "node:http";
import crypto from "node:crypto";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const baseUrl = process.env.CN_BASE_URL ?? "http://127.0.0.1:5212";
const adminEmail = process.env.CN_ADMIN_EMAIL ?? "admin@example.com";
const adminPassword = process.env.CN_ADMIN_PASSWORD ?? "";
const memberEmail = process.env.CN_MEMBER_EMAIL ?? "member@example.com";
const memberPassword = process.env.CN_MEMBER_PASSWORD ?? "";
const stamp = Date.now();
const providerId = 3;
const port = 19090;
const issuer = `http://127.0.0.1:${port}`;
const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dbPath = process.env.CN_DB_PATH ?? path.join(repoRoot, "runtime", "data", "cloudreve.db");
const pwshPath =
  process.env.CN_PWSH ??
  "C:\\Users\\Baixiong\\.cache\\codex-runtimes\\codex-primary-runtime\\dependencies\\native\\powershell\\pwsh.exe";
const oidcSecret = "ce-secret";
const logtoSecret = `ce-logto-secret-${stamp}`;

const checks = [];
const check = (name, ok, detail = "") => {
  checks.push({ name, ok, detail });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? `  ${detail}` : ""}`);
  if (!ok) process.exitCode = 1;
};

const api = async (method, path, { token, body } = {}) => {
  const headers = { "Content-Type": "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;
  const resp = await fetch(`${baseUrl}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    redirect: "manual",
  });
  const text = await resp.text();
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    json = undefined;
  }
  return { status: resp.status, json, text, headers: resp.headers };
};

const expectOk = (resp, label) => {
  if (!resp.json || resp.json.code !== 0) {
    throw new Error(`${label}: code=${resp.json?.code} msg=${resp.json?.msg} raw=${resp.text.slice(0, 400)}`);
  }
  return resp.json.data;
};

const { privateKey, publicKey } = crypto.generateKeyPairSync("rsa", { modulusLength: 2048 });
const jwk = publicKey.export({ format: "jwk" });
const kid = "ce-sso-test";
const b64url = (input) => Buffer.from(input).toString("base64url");
const tokenSecrets = new Map();

const sqlLiteral = (value) => `'${String(value).replaceAll("'", "''")}'`;

const runSqlScript = (script, sql) => {
  const result = spawnSync(
    pwshPath,
    ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path.join(repoRoot, "tools", script), "-Database", dbPath, "-Sql", sql],
    { encoding: "utf8" },
  );
  if (result.error) {
    throw new Error(`failed to run ${script}: ${result.error.message}`);
  }
  if (result.status !== 0) {
    throw new Error(`${script} failed (${result.status}): ${result.stderr || result.stdout}`);
  }
  return result.stdout.trim();
};

const insertLegacyIdentity = ({ userID, subject, email }) => {
  runSqlScript(
    "sqlite-exec.ps1",
    `INSERT INTO external_identities (created_at, updated_at, provider, subject, email, display_name, avatar, user_id)
VALUES (CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'oidc', ${sqlLiteral(subject)}, ${sqlLiteral(email)}, 'Legacy Linked', NULL, ${Number(userID)})`,
  );
};

const querySql = (sql) => runSqlScript("sqlite-query-utf8.ps1", sql);

const signIdToken = (claims) => {
  const header = b64url(JSON.stringify({ alg: "RS256", typ: "JWT", kid }));
  const payload = b64url(JSON.stringify(claims));
  const signature = crypto.sign("RSA-SHA256", Buffer.from(`${header}.${payload}`), privateKey);
  return `${header}.${payload}.${signature.toString("base64url")}`;
};

// Each authorization code carries the identity to return. The nonce is also
// the code so the test can drive the flow without a browser.
const profiles = new Map();

const server = http.createServer((req, resp) => {
  const url = new URL(req.url, issuer);
  const sendJson = (body) => {
    resp.writeHead(200, { "Content-Type": "application/json" });
    resp.end(JSON.stringify(body));
  };

  if (url.pathname === "/.well-known/openid-configuration") {
    return sendJson({
      issuer,
      authorization_endpoint: `${issuer}/authorize`,
      token_endpoint: `${issuer}/token`,
      userinfo_endpoint: `${issuer}/userinfo`,
      jwks_uri: `${issuer}/jwks`,
      scopes_supported: ["openid", "email", "profile"],
      id_token_signing_alg_values_supported: ["RS256"],
    });
  }
  if (url.pathname === "/jwks") {
    return sendJson({
      keys: [{ kty: "RSA", kid, use: "sig", alg: "RS256", n: jwk.n, e: jwk.e }],
    });
  }
  if (url.pathname === "/token" && req.method === "POST") {
    let raw = "";
    req.on("data", (chunk) => (raw += chunk));
    req.on("end", () => {
      const form = new URLSearchParams(raw);
      const code = form.get("code") ?? "";
      const profile = profiles.get(code);
      tokenSecrets.set(code, form.get("client_secret") ?? "");
      if (!profile) {
        resp.writeHead(400, { "Content-Type": "application/json" });
        return resp.end(JSON.stringify({ error: "invalid_grant" }));
      }
      const now = Math.floor(Date.now() / 1000);
      sendJson({
        access_token: `access-${code}`,
        token_type: "Bearer",
        expires_in: 300,
        id_token: signIdToken({
          iss: issuer,
          aud: profile.clientId,
          sub: profile.sub,
          nonce: code,
          email: profile.email,
          email_verified: true,
          name: profile.name,
          picture: profile.picture,
          iat: now,
          exp: now + 300,
        }),
      });
    });
    return;
  }
  if (url.pathname === "/userinfo") {
    const auth = req.headers.authorization ?? "";
    const code = auth.replace(/^Bearer access-/, "");
    const profile = profiles.get(code);
    if (!profile) {
      resp.writeHead(401);
      return resp.end();
    }
    return sendJson({
      sub: profile.sub,
      email: profile.email,
      email_verified: true,
      name: profile.name,
      picture: profile.picture,
    });
  }
  resp.writeHead(404);
  resp.end();
});

await new Promise((resolve) => server.listen(port, "127.0.0.1", resolve));

let adminToken;
let previousProviders;
const configure = async (providers) => {
  const body = { settings: { external_login_providers: JSON.stringify(providers) } };
  expectOk(await api("PATCH", "/api/v4/admin/settings", { token: adminToken, body }), "save providers");
};

try {
  adminToken = expectOk(
    await api("POST", "/api/v4/session/token", {
      body: { email: adminEmail, password: adminPassword },
    }),
    "admin login",
  ).token.access_token;

  const settings = expectOk(
    await api("POST", "/api/v4/admin/settings", {
      token: adminToken,
      body: { keys: ["external_login_providers"] },
    }),
    "read providers",
  );
  previousProviders = settings?.external_login_providers;

  const discovery = await (await fetch(`${issuer}/.well-known/openid-configuration`)).json();
  const providerConfig = {
    version: 1,
    next_id: 4,
    providers: [
      {
        id: providerId,
        type: "oidc",
        enabled: true,
        config: {
          direct_sign_in: true,
          display_name: "CN Test SSO",
          icon: "mdi:openid",
          scope: "email",
          client_id: `ce-client-${stamp}`,
          client_secret: oidcSecret,
          sub_field_mapping: "sub",
          email_field_mapping: "email",
          avatar_field_mapping: "picture",
          name_field_mapping: "name",
        },
        wellknown: discovery,
      },
      {
        id: 0,
        type: "logto",
        enabled: true,
        config: {
          direct_sign_in: true,
          display_name: "CN Logto Zero",
          scope: "email",
          endpoint: issuer,
          client_id: `ce-logto-${stamp}`,
          client_secret: logtoSecret,
        },
        wellknown: discovery,
      },
    ],
  };
  await configure(providerConfig);

  // Secrets are redacted on read, and saving the redacted payload must keep
  // the original secret instead of overwriting it with "***".
  const afterSave = expectOk(
    await api("POST", "/api/v4/admin/settings", {
      token: adminToken,
      body: { keys: ["external_login_providers"] },
    }),
    "read redacted providers",
  );
  const redacted = JSON.parse(afterSave.external_login_providers);
  const redactedOIDC = redacted.providers?.find((item) => item.id === providerId);
  const redactedLogto = redacted.providers?.find((item) => item.id === 0);
  check(
    "client secret is redacted on read",
    redactedOIDC?.config?.client_secret === "***" &&
      redactedLogto?.config?.client_secret === "***",
    JSON.stringify([redactedOIDC?.config?.client_secret, redactedLogto?.config?.client_secret]),
  );
  check("provider id is preserved on read", redactedOIDC?.id === providerId && redactedLogto?.id === 0);
  await configure(redacted);

  // Logto is allowed to use an id other than 0, matching Pro's next_id
  // allocation, and ids must survive a save/read round trip.
  const idsConfig = structuredClone(providerConfig);
  idsConfig.providers.push({
    id: 7,
    type: "logto",
    enabled: false,
    config: { display_name: "Logto Seven", endpoint: "https://logto.invalid" },
  });
  await configure(idsConfig);
  const idsRead = JSON.parse(
    expectOk(
      await api("POST", "/api/v4/admin/settings", {
        token: adminToken,
        body: { keys: ["external_login_providers"] },
      }),
      "read provider ids",
    ).external_login_providers,
  );
  check(
    "logto keeps non-zero provider id",
    !!idsRead.providers?.some((item) => item.type === "logto" && item.id === 7),
    JSON.stringify(idsRead.providers?.map((item) => [item.type, item.id])),
  );
  await configure(providerConfig);

  const loginConfig = expectOk(await api("GET", "/api/v4/site/config/login"), "site login config");
  const listed = (loginConfig.login_providers ?? []).find((item) => item.id === providerId);
  check("provider exposed on login page", !!listed && listed.enabled && listed.icon === "mdi:openid");

  const runFlow = async ({ token, linking, profile, provider = providerId }) => {
    const start = expectOk(
      await api("PUT", "/api/v4/session/openid", {
        token: linking ? token : undefined,
        body: { provider, linking: !!linking, hint: "/setting?tab=security" },
      }),
      "start openid",
    );
    const authUrl = new URL(start);
    const state = authUrl.searchParams.get("state");
    const nonce = authUrl.searchParams.get("nonce");
    const redirectUri = authUrl.searchParams.get("redirect_uri");
    check("authorization url is provider scoped", redirectUri.endsWith(`/callback/openid/${provider}`), redirectUri);
    check("pkce challenge present", !!authUrl.searchParams.get("code_challenge"));
    check(
      "pro callback uses frontend route",
      redirectUri.endsWith(`/callback/openid/${provider}`),
      redirectUri,
    );
    profiles.set(nonce, { ...profile, clientId: authUrl.searchParams.get("client_id") });
    return { state, nonce, redirectUri, provider };
  };

  const findAdminUser = async (email) => {
    const data = expectOk(
      await api("POST", "/api/v4/admin/user", {
        token: adminToken,
        body: {
          page: 1,
          page_size: 50,
          order_by: "id",
          order_direction: "desc",
          conditions: { user_email: email },
        },
      }),
      "find admin user",
    );
    return (data.users ?? []).find((item) => item.email === email);
  };

  const checkAdminLink = async (email, expectedProvider, label) => {
    const adminUser = await findAdminUser(email);
    const links = adminUser?.edges?.openid ?? [];
    const link = links.find((item) => item.provider === expectedProvider);
    check(
      label,
      !!link && !!link.linked_at && !!link.sub,
      JSON.stringify({ provider: link?.provider, linked_at: link?.linked_at }),
    );
    return link;
  };

  // Logto provider id 0 must not be mistaken for "no provider selected".
  const logtoEmail = `logto-zero-${stamp}@test.local`;
  const logtoFlow = await runFlow({
    provider: 0,
    profile: {
      sub: `logto-sub-${stamp}`,
      email: logtoEmail,
      name: "Logto Zero",
      picture: "",
    },
  });
  const logtoExchange = expectOk(
    await api("POST", "/api/v4/session/openid", {
      body: { provider_id: 0, code: logtoFlow.nonce, session_id: logtoFlow.state },
    }),
    "exchange logto provider 0",
  );
  check("logto provider id 0 signs in", logtoExchange.user?.email === logtoEmail, logtoExchange.user?.email);
  check(
    "logto received original secret after redacted save",
    tokenSecrets.get(logtoFlow.nonce) === logtoSecret,
    tokenSecrets.get(logtoFlow.nonce),
  );
  await checkAdminLink(logtoEmail, 0, "admin lists logto provider 0 link with linked_at");

  // First sign-in: direct sign-in creates a local account.
  const email = `sso-first-${stamp}@test.local`;
  const first = await runFlow({
    profile: { sub: `sub-${stamp}`, email, name: "SSO First", picture: "https://example.com/a.png" },
  });
  profiles.get(first.nonce).email = email;
  const exchange = expectOk(
    await api("POST", "/api/v4/session/openid", {
      body: { provider_id: providerId, code: first.nonce, session_id: first.state },
    }),
    "exchange callback",
  );
  check("first sign-in created account", exchange.user?.email === email, exchange.user?.email);
  check(
    "oidc received original secret after redacted save",
    tokenSecrets.get(first.nonce) === oidcSecret,
    tokenSecrets.get(first.nonce),
  );
  const firstAdminLink = await checkAdminLink(email, providerId, "admin lists OIDC link with provider id");
  check("admin OIDC link exposes subject", !!firstAdminLink?.sub, firstAdminLink?.sub ?? "");

  const replay = await api("POST", "/api/v4/session/openid", {
    body: { provider_id: providerId, code: first.nonce, session_id: first.state },
  });
  check("state replay rejected", replay.json?.code !== 0, `code=${replay.json?.code}`);
  const codeReplayStart = await runFlow({
    profile: { sub: `sub-${stamp}`, email, name: "SSO First", picture: "" },
  });
  const codeReplay = await api("POST", "/api/v4/session/openid", {
    body: { provider_id: providerId, code: first.nonce, session_id: codeReplayStart.state },
  });
  check("authorization code replay rejected", codeReplay.json?.code !== 0, `code=${codeReplay.json?.code}`);

  // Second sign-in returns the same account.
  const second = await runFlow({
    profile: { sub: `sub-${stamp}`, email, name: "SSO First", picture: "" },
  });
  const exchange2 = expectOk(
    await api("POST", "/api/v4/session/openid", {
      body: { provider_id: providerId, code: second.nonce, session_id: second.state },
    }),
    "exchange second callback",
  );
  check("returning sign-in reuses account", exchange2.user?.email === email, exchange2.user?.email);

  // Legacy OIDC endpoints keep their original API callback and ticket flow.
  const legacyStart = expectOk(
    await api("POST", "/api/v4/session/sso/oidc", {
      body: { redirect: "/setting?tab=security" },
    }),
    "legacy oidc start",
  );
  const legacyUrl = new URL(legacyStart.url);
  const legacyState = legacyUrl.searchParams.get("state");
  const legacyNonce = legacyUrl.searchParams.get("nonce");
  check(
    "legacy callback keeps api route",
    legacyUrl.searchParams.get("redirect_uri") === `${baseUrl}/api/v4/session/sso/oidc/callback`,
    legacyUrl.searchParams.get("redirect_uri") ?? "",
  );
  profiles.set(legacyNonce, {
    sub: `legacy-sub-${stamp}`,
    email: `legacy-${stamp}@test.local`,
    name: "Legacy SSO",
    picture: "",
    clientId: legacyUrl.searchParams.get("client_id"),
  });
  const legacyCallback = await api(
    "GET",
    `/api/v4/session/sso/oidc/callback?code=${encodeURIComponent(legacyNonce)}&state=${encodeURIComponent(legacyState)}`,
  );
  const legacyLocation = legacyCallback.headers.get("location") ?? "";
  check("legacy callback redirects with ticket", legacyLocation.includes("ticket="), legacyLocation);
  const legacyTicket = new URL(legacyLocation).searchParams.get("ticket");
  const legacyExchange = expectOk(
    await api("POST", "/api/v4/session/sso/oidc/exchange", { body: { ticket: legacyTicket } }),
    "legacy ticket exchange",
  );
  check("legacy flow signs in", legacyExchange.user?.email === `legacy-${stamp}@test.local`);

  // Linking to an existing member account.
  const memberToken = expectOk(
    await api("POST", "/api/v4/session/token", {
      body: { email: memberEmail, password: memberPassword },
    }),
    "member login",
  ).token.access_token;

  const linkProviderId = providerId + 1;
  providerConfig.next_id = linkProviderId + 1;
  providerConfig.providers.push({
    id: linkProviderId,
    type: "oidc",
    enabled: true,
    config: {
      direct_sign_in: false,
      display_name: "CN Link SSO",
      scope: "email",
      client_id: `ce-link-client-${stamp}`,
      client_secret: "ce-link-secret",
    },
    wellknown: discovery,
  });
  await configure(providerConfig);

  const linkStart = expectOk(
    await api("PUT", "/api/v4/session/openid", {
      token: memberToken,
      body: { provider: linkProviderId, linking: true, hint: "/setting?tab=security" },
    }),
    "start link",
  );
  const linkUrl = new URL(linkStart);
  const linkState = linkUrl.searchParams.get("state");
  const linkNonce = linkUrl.searchParams.get("nonce");
  profiles.set(linkNonce, {
    sub: `link-sub-${stamp}`,
    email: memberEmail,
    name: "Member Linked",
    picture: "",
    clientId: linkUrl.searchParams.get("client_id"),
  });

  const linkExchange = await api("POST", "/api/v4/session/openid", {
    body: { provider_id: linkProviderId, code: linkNonce, session_id: linkState },
  });
  check("link callback succeeds", linkExchange.json?.code === 0, `code=${linkExchange.json?.code} msg=${linkExchange.json?.msg}`);

  const memberSettings = expectOk(
    await api("GET", "/api/v4/user/setting", { token: memberToken }),
    "member settings",
  );
  const linked = (memberSettings.open_id ?? []).find((item) => item.provider === linkProviderId);
  check("open_id lists linked provider", !!linked, JSON.stringify(memberSettings.open_id ?? []));
  check("open_id exposes linked_at", !!linked?.linked_at, linked?.linked_at ?? "");

  // Repeated linking of the same provider must not duplicate the row.
  const relinkStart = expectOk(
    await api("PUT", "/api/v4/session/openid", {
      token: memberToken,
      body: { provider: linkProviderId, linking: true, hint: "/setting?tab=security" },
    }),
    "start relink",
  );
  const relinkUrl = new URL(relinkStart);
  const relinkState = relinkUrl.searchParams.get("state");
  const relinkNonce = relinkUrl.searchParams.get("nonce");
  profiles.set(relinkNonce, {
    sub: `link-sub-${stamp}`,
    email: memberEmail,
    name: "Member Linked",
    picture: "",
    clientId: relinkUrl.searchParams.get("client_id"),
  });
  await api("POST", "/api/v4/session/openid", {
    body: { provider_id: linkProviderId, code: relinkNonce, session_id: relinkState },
  });
  const relinked = expectOk(
    await api("GET", "/api/v4/user/setting", { token: memberToken }),
    "settings after relink",
  );
  check(
    "relink does not duplicate provider",
    (relinked.open_id ?? []).filter((item) => item.provider === linkProviderId).length === 1,
    JSON.stringify(relinked.open_id ?? []),
  );

  // Cross-user binding must be rejected.
  const conflictStart = expectOk(
    await api("PUT", "/api/v4/session/openid", {
      token: memberToken,
      body: { provider: providerId, linking: true, hint: "/setting?tab=security" },
    }),
    "start conflict link",
  );
  const conflictUrl = new URL(conflictStart);
  const conflictState = conflictUrl.searchParams.get("state");
  const conflictNonce = conflictUrl.searchParams.get("nonce");
  profiles.set(conflictNonce, {
    sub: `sub-${stamp}`,
    email: `conflict-${stamp}@test.local`,
    name: "Conflict",
    picture: "",
    clientId: conflictUrl.searchParams.get("client_id"),
  });
  const conflictCallback = await api("POST", "/api/v4/session/openid", {
    body: { provider_id: providerId, code: conflictNonce, session_id: conflictState },
  });
  check(
    "cross-user identity binding rejected",
    conflictCallback.json?.code !== 0,
    `code=${conflictCallback.json?.code} msg=${conflictCallback.json?.msg}`,
  );

  // Unlink and verify both stores are clean.
  const unlink = await api("DELETE", `/api/v4/session/openid/${linked.id}`, { token: memberToken });
  check("unlink succeeds", unlink.json?.code === 0, `code=${unlink.json?.code} msg=${unlink.json?.msg}`);
  const afterUnlink = expectOk(
    await api("GET", "/api/v4/user/setting", { token: memberToken }),
    "settings after unlink",
  );
  check(
    "unlink removes open_id entry",
    !(afterUnlink.open_id ?? []).some((item) => item.provider === linkProviderId),
  );

  // Rows written by earlier builds used the bare provider type. A normal
  // sign-in should migrate them in place to the provider-scoped key.
  const legacyEmail = `legacy-row-${stamp}@test.local`;
  const legacyPassword = "{{SECRET_4bd4dxw5}}";
  expectOk(
    await api("POST", "/api/v4/user", {
      body: { email: legacyEmail, password: legacyPassword, language: "zh-CN" },
    }),
    "create legacy-row user",
  );
  const legacyAdminUser = await findAdminUser(legacyEmail);
  if (!legacyAdminUser) {
    throw new Error("legacy-row user was not found through the admin API");
  }

  const legacySubject = `legacy-row-sub-${stamp}`;
  insertLegacyIdentity({
    userID: legacyAdminUser.id,
    subject: legacySubject,
    email: legacyEmail,
  });
  const beforeRows = querySql(
    `SELECT provider, subject, user_id FROM external_identities WHERE subject=${sqlLiteral(legacySubject)};`,
  );
  check("legacy bare identity fixture inserted", beforeRows.includes(`oidc|${legacySubject}|`), beforeRows);

  const legacyRowFlow = await runFlow({
    profile: {
      sub: legacySubject,
      email: `legacy-login-${stamp}@test.local`,
      name: "Legacy Row",
      picture: "",
    },
  });
  const legacyRowExchange = expectOk(
    await api("POST", "/api/v4/session/openid", {
      body: { provider_id: providerId, code: legacyRowFlow.nonce, session_id: legacyRowFlow.state },
    }),
    "exchange legacy-row login",
  );
  check("legacy identity resolves to existing user", legacyRowExchange.user?.email === legacyEmail, legacyRowExchange.user?.email);

  const afterRows = querySql(
    `SELECT provider, subject, user_id FROM external_identities WHERE subject=${sqlLiteral(legacySubject)};`,
  );
  const expectedProviderKey = `oidc#${providerId}`;
  check(
    "legacy identity migrated to provider-scoped key",
    afterRows.includes(`${expectedProviderKey}|${legacySubject}|${legacyAdminUser.id}`),
    afterRows,
  );
  check("legacy bare identity no longer exists", !afterRows.split(/\r?\n/).some((row) => row.startsWith(`oidc|${legacySubject}|`)), afterRows);
} finally {
  if (adminToken) {
    const restore = previousProviders
      ? previousProviders
      : JSON.stringify({ version: 1, next_id: 3, providers: [] });
    await api("PATCH", "/api/v4/admin/settings", {
      token: adminToken,
      body: { settings: { external_login_providers: restore } },
    });
  }
  server.close();
  const failed = checks.filter((item) => !item.ok).length;
  console.log(`\n${checks.length - failed}/${checks.length} checks passed`);
}
