# 已补齐的功能

本分支在上游社区版源码上补齐下列能力，接口与数据结构尽量与 Pro 对齐。

## 滥用举报

- 用户端：`POST /api/v4/site/abuse`，支持举报用户或分享；保留 `POST /api/v4/share/report/:id`
- 管理端：`GET /api/v4/admin/abuse`、`GET /api/v4/admin/abuse/:id`、`POST /api/v4/admin/abuse/batch/delete`
- 软删除、筛选键、HashID 与数字 ID 双兼容、验证码开关

## 审计日志 / 事件

- `POST /api/v4/admin/event` 列表与筛选
- 记录文件/目录创建、重命名、删除等操作，落库为 `audit_logs`

## SSO / OpenID Connect

- 授权码 + PKCE（S256）+ state + nonce
- 一次性 ticket 兑换：`POST /api/v4/session/sso/oidc/exchange`
- 用户侧绑定/解绑：`GET/POST/DELETE /api/v4/user/sso/oidc`
- 仅当 `email_verified` 为真时才按邮箱关联既有账号

## 增值服务

- 积分、礼品码、存储包、订单与支付回调
- 管理端：`/admin/vas`、`/admin/payment`、`/admin/event`
- 用户端：`/shop`、`/settings?tab=finance`

## 分享 ACL

- `GET/PUT /api/v4/share/:id/acl`
- 用户、用户组、内置受众三类规则，`/share/info` 对无权访问者脱敏

## 组多存储策略 / 负载均衡

- `GET/PUT /api/v4/admin/group/:id/policies`
- `GET /api/v4/user/setting/policies` 按组绑定返回可用策略
- 存储策略支持负载均衡节点组

## 工作流

- 归档、解压、导入、迁移统一走 `service/explorer/workflows.go` 的校验

## 有意未实现

- Pro 的授权/许可证购买与管理（`/admin/home` 的授权卡片）——本分支不销售授权，
  也不会调用任何 Pro 授权服务。
- 任何依赖 Pro 闭源二进制的功能。
