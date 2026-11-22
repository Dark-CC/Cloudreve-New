# 1yunpan edge upload

`upload.asnndi.com` 由一段 Cloudflare Snippet 提供，浏览器把分片直接发到这里，
边缘解密 Cloudreve 签发的票据后转发到 1yunpan 上传节点。浏览器和中转节点都不
接触 1yunpan 账号。

## 为什么不需要在边缘写账号

- Cloudreve 每次创建上传会话时，用 AES-256-GCM 把 `phpsessid`、目标路径、文件
  大小、上传节点和过期时间封成票据。
- 边缘只持有「票据密钥」，解密后拿到当次会话；密钥和账号是两回事。
- 同一份 Snippet + 同一个密钥可以服务多个站点，因此 `upload.asnndi.com` 可以
  作为公共上传域名给别的实例复用。没有有效票据的请求只会得到 403。

## 部署

1. 生成密钥（与 Cloudreve 策略里的「票据密钥」保持一致）：

   ```bash
   openssl rand -base64 32
   ```

2. 用脚本生成已内联密钥的版本，**不要手工改密钥**（少一个 `=`、多一个空格都会
   解不开票据）：

   ```bash
   node make-snippet.mjs '<票据密钥>' --min --out snippet.ready.js
   ```

   把 `snippet.ready.js` 整个粘进控制台即可。`--site mysite` 会同时写入站点条目；
   不加则只用默认条目。
3. Cloudflare 控制台 → 你的 Zone → Rules → Snippets → 创建 Snippet：
   - 触发规则：`http.host eq "upload.asnndi.com"`
   - 代码：粘贴 `snippet.js`
4. 为 `upload.asnndi.com` 添加一条 `AAAA 100::` 的代理（橙色云）DNS 记录，让
   请求进入 Cloudflare 边缘。
5. Cloudreve 管理端 → 存储策略 → 1yunpan：
   - 边缘上传地址：`https://upload.asnndi.com`
   - 票据密钥：与 `TICKET_KEYS` 中对应条目相同
   - 边缘站点标识：可选，填 `mysite` 时使用 `TICKET_KEYS["mysite"]`
6. 保存后策略的 `relay` 会自动关闭，新上传走浏览器 → 边缘 → 1yunpan。

## 保存时提示「未知错误」

> 校验密钥是否一致（不传密钥本身，只比对指纹）：
>
> ```bash
> node tools/oneyunpan-edge/check-edge.mjs https://upload.asnndi.com '<票据密钥>'
> ```
>
> 输出 `MATCH` 才能正常上传；`MISMATCH` 说明边缘里的密钥和策略不是同一个。
> 如果边缘没有返回 JSON，说明线上还是旧版本，重新粘贴一次即可。

Cloudflare 控制台保存 Snippet 时只回一句 `unknown error`，不告诉你哪一行出错。
按下面顺序排查，基本能定位到具体原因：

1. **先贴最小片段**：把 `minimal-test.js` 整个粘进一个新的 Snippet 保存。
   - 能保存、访问 `https://upload.asnndi.com/` 返回 `ok`：说明 Snippet 可用，问题在
     完整脚本，跳到第 3 步。
   - 仍然报未知错误：说明这个 Zone 没开通 Snippets（需要 Pro 及以上，且该功能要
     单独灰度），改用下面的 Worker 方案。
2. **确认触发规则和 DNS**：Snippets 只在触发规则命中时才允许保存规则；主机名必须
   有一条橙色云代理记录，否则保存或生效都会失败。
3. **常见代码原因**：
   - 脚本里出现 `import` / `require` / `process` / `Buffer` / `node:`，Snippets
     不支持；
   - 使用了 `duplex: "half"`、顶层 `await`、或 `crypto.subtle.importKey` 之外的
     加密 API；
   - 密钥不是 32 字节 base64，脚本加载时就会抛错。
   - 源文件含非 ASCII 字符（中文注释等）。部分控制台会因此直接报未知错误，
     当前两个脚本都已保持纯 ASCII，中文匹配改用 `\u` 转义。
   - 脚本超过 Snippets 的体积上限。`snippet.js` 约 8 KB，`snippet.min.js` 约
     4 KB，先用小的那个。

当前 `snippet.js` 已避开以上写法，只依赖 `fetch`、`Headers`、`Response`、
`crypto.subtle`、`atob` 和 `TextDecoder`。

## Worker 部署（Snippets 不可用时）

Worker 的免费额度足够这个用途，配置一次即可，后续改代码用
`npx wrangler deploy` 发布：

```bash
cd tools/oneyunpan-edge
npx wrangler login
npx wrangler deploy
# 密钥通过 Secret 注入，不用改代码
echo '{"":"<base64 32-byte key>"}' | npx wrangler secret put TICKET_KEYS
```

`wrangler.toml` 已把 `upload.asnndi.com/*` 绑定到这个 Worker。若之前建过 Snippet，
请先停用它的触发规则，避免同一主机被两个入口处理。

`worker.js` 是**由 `build-worker.mjs` 从 `snippet.js` 生成**的完整单文件，不要再让它
`import` 别的模块。Cloudflare 在给 Worker 打包时会重命名资源（你看到的
`226004/snippet.js`），一旦被引用的文件没有 `export default`，运行时就会报
`does not provide an export named 'default'`。改完 `snippet.js` 后执行：

```bash
node build-worker.mjs
```

Worker 版直接读 `TICKET_KEYS` 环境变量（JSON），所以轮换密钥只需更新 Secret，
不用重新发布代码。多个站点共用同一域名时，写成
`{"":"默认密钥","siteA":"站点 A 密钥","siteB":"站点 B 密钥"}` 即可。

验证：`curl https://upload.asnndi.com/` 应返回 `ok`。

## 会话过期

1yunpan 在登录失效时的表现在两种情况下不同：

- 列表/API 请求返回 `401`；
- 末尾分片仍返回 `200`，但响应体是 `{"status":true,"callback":{"status":false,
  "message":"无效的认证令牌-1"}}`。

边缘把两种情况都识别为会话失效，返回 `401` 和 `X-Cr-Session-Expired: 1`。浏览器据此调用
`POST /api/v4/callback/oneyunpan/{sessionID}/{secret}/refresh` 换取新票据，并从
第 0 字节重新开始链式上传。Cloudreve 同时顺延上传会话 TTL，因此长时间上传不会
在中途失效。

公共上传链接（`/upload/<token>`）没有登录态，同样走这套续票流程，但调用的是
`POST /api/v4/upload-link/{token}/refresh/{sessionID}/{secret}`，凭会话回调密钥授权。

## 限制

- 每次浏览器请求只产生 1 次子请求，符合 Snippets Pro 的 2 次上限。
- 首片必须是 1 字节，由浏览器上传器负责切分；边缘不接受浏览器自报的路径。
- Snippets 不能读取 Secret，所以密钥写在 Snippet 源码里；轮换时需同时更新
  Cloudreve 策略。
- 多个实例共用 `upload.asnndi.com` 时，各自使用不同的「边缘站点标识 + 密钥」，
  互不需要信任对方的密钥。
- 文件最小 2 字节，这是 1yunpan 的限制。
