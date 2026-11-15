# 端到端测试脚本

这些脚本针对一个**正在运行的测试实例**执行 HTTP 断言，不会修改源码。

## 前置条件

1. 启动一个测试实例（建议独立数据目录，勿指向生产库）：

   ```bash
   ./cloudreve -c data/conf.ini server
   ```

2. 准备一个管理员账号，并通过环境变量提供凭据。脚本内**不包含**任何默认密码：

   ```powershell
   $env:CN_ADMIN_EMAIL    = "admin@example.com"
   $env:CN_ADMIN_PASSWORD = "<your-test-password>"
   $env:CN_MEMBER_EMAIL   = "member@example.com"
   $env:CN_MEMBER_PASSWORD= "<your-test-password>"
   ```

3. 通过 `-BaseUrl` 指定地址，默认 `http://127.0.0.1:5212`。

## 脚本

| 脚本 | 覆盖范围 |
| --- | --- |
| `cn-delivery-check.ps1` | 关键页面可访问性、组设置、PWA 清单 |
| `cn-registration-check.ps1` | 新用户注册与登录 |
| `cn-pro-runtime-test.ps1` | 组策略、根目录策略、负载均衡等运行态 |
| `cn-acl-test.ps1` / `cn-acl-hashid-test.ps1` | 分享 ACL 与 `/share/info` 脱敏 |
| `cn-shared-with-me-acl-test.ps1` | "共享给我"的权限隔离 |
| `cn-audit-archive-test.ps1` | 审计归档、下载、按类别清理 |
| `cn-giftcode-test.ps1` | 礼品码生成、编辑、兑换、防重用 |
| `cn-payment-*.ps1` | 支付网关配置、回调、事件 |
| `cn-sso-test.mjs` | OIDC 多 Provider、密钥脱敏、身份迁移 |
| `cn-import-test.ps1` | 管理员导入与越权隔离 |
| `cn-license-test.ps1` | 本地离线授权摘要与校验 |

## 注意

- 这些脚本会创建测试用户/分享/订单，**不要对生产实例运行**。
- `cn-sso-test.mjs` 需要在仓库根目录执行，以便定位 `runtime/data` 下的测试库。
