# 架构与实现说明

## 1. 角色

插件在 CLIProxyAPI 内核进程内运行，通过 C ABI v1 / Schema v10 与宿主交换 JSON-RPC 消息。它不启动独立服务，也不监听端口。

```
┌────────────┐   plugin.register / model.static / auth.* / executor.*   ┌──────────────┐
│ CPA 内核   │ ───────────────────────────────────────────────────────▶ │ qwenwork.dll │
│ (Go)       │ ◀─────────────────────────────────────────────────────── │ (Go c-shared)│
└────────────┘        JSON over C ABI（host.call 回调用于流式）          └──────────────┘
```

## 2. 注册的能力

| 能力 | 处理函数 |
|---|---|
| `model_provider` | `model.static`、`model.for_auth` |
| `auth_provider` | `auth.identifier`、`auth.parse`、`auth.login.start/poll`、`auth.refresh` |
| `executor` | `executor.identifier`、`executor.execute`、`executor.execute_stream` |
| `quota_provider` | `quota.identifier/describe/fetch` |
| `management_api` | `plugin.management.register/handle`，暴露 `/panel` 与 `/icon.png` |

## 3. 鉴权模型

### 3.1 OAuth 设备码（多账号）

1. `auth.login.start`：生成 PKCE（S256）与 nonce/machine_id，返回
   `https://gateway.qwenwork.cn/device/selectAccounts?...`；
2. 服务端 302 到 `qwenwork.cn/oauth2/auth`，用户在浏览器完成登录；
3. `auth.login.poll`：`GET /api/v1/deviceToken/poll?nonce=&verifier=&challenge_method=S256`，
   404 表示等待，200 返回 `token` / `refresh_token` / 过期时间；
4. 插件用 token 调 `/api/v1/adapter/user/account-context` 取 uid 与昵称；
5. 返回 `AuthData`，`FileName` 固定为 `qwenwork-<uid>.json`，内核落盘到 `oauth/`。

`auth.refresh` 用 refresh_token 调 `/api/v1/deviceToken/refresh`，只发送 refresh_token（避免 access token 失效干扰）。

### 3.2 本机登录态（单机导入）

`auth.parse` 接受 `auth_kind: local` 的 JSON，只记录 `auth_dir`。执行请求时实时读取：

```
Local State (DPAPI) → 32B AES key → auth-v2.dat (v10, AES-256-GCM) → JWT
```

不复制文件、不刷新 refresh token，因此不会与客户端互相踢下线。

## 4. 模型调用

```
executor.request
  → credentialFromStorage(storage_json)       // OAuth 或本机
  → POST {origin}/api/desktop-sessions        // 2h 票，按 token 缓存
  → POST {base}/v1/chat/completions
      headers: Authorization: Bearer sk-mm-…
               Accept-Encoding: identity      // 关键：避免 gzip 头错配
  → 非流式：整个 JSON 回传
  → 流式：逐行读取 SSE，经 host.stream.emit 推送，最后 host.stream.close
```

注意：`executor.execute_stream` 不能直接返回 `chan`，必须走 `host.stream.emit`，否则内核报
`json: unsupported type: <-chan ...`。

## 5. 额度口径

| 指标 | 来源 | 说明 |
|---|---|---|
| 总额度百分比 | `/api/v2/quota/usage.total_usage_percentage` | 真实已用百分比，剩余 = 1 − used% |
| 剩余额度 | `/api/v1/adapter/user/account-context?include=quota` | `quota.remaining` |
| 月度/长期/每日 | `/api/v1/adapter/user/wallets` | 只有余额，没有上限字段 |

个人免费版 `quota.total/used` 为 `null`，`/user/balance` 也只返回余额，因此月度/长期/每日不做百分比。

## 6. 管理面板

`management_api` 注册两个 resource 路由：

- `/v0/resource/plugins/qwenwork/panel`：服务端渲染的 HTML，展示账号、三档额度、模型列表；
- `/v0/resource/plugins/qwenwork/icon.png`：128×128 圆角图标（内核插件 logo 只接受 http(s) URL）。

面板不经过管理密钥鉴权，但只在 CPA 本机监听地址上可访问。

## 7. 失败与重试

| 场景 | 行为 |
|---|---|
| 票过期 | 401/403 时清缓存并重取一次 |
| 429 | 原样返回上游错误，调用方退避（免费号 4 次/分钟） |
| 登录态缺失 | `auth_error`，提示先在客户端登录或添加账号 |
| 上游 5xx | 透传状态码与响应体前 600 字节 |

## 8. 目录

```
cmd/qwenwork-plugin/main.go   C ABI 导出（init/call/free/shutdown）
internal/cpa/plugin.go        注册与能力声明
internal/cpa/auth.go          OAuth 设备码、解析、刷新
internal/cpa/token.go         DPAPI 解密、换票、缓存
internal/cpa/executor.go      chat-completions 执行与流式
internal/cpa/quota.go         额度查询
internal/cpa/panel.go         管理面板与图标路由
internal/pluginabi            宿主 ABI 常量
internal/pluginapi            宿主 RPC 类型
```

