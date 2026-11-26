# QWork OAuth (cpa-plugin-qwork-oauth)

[![Release](https://img.shields.io/github/v/release/wangxiaoshuai136/cpa-plugin-qwork-oauth)](https://github.com/wangxiaoshuai136/cpa-plugin-qwork-oauth/releases)
[![License](https://img.shields.io/github/license/wangxiaoshuai136/cpa-plugin-qwork-oauth)](LICENSE)

把 **千问办公（QwenWork）** 接入 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 的原生插件（C ABI v1 / Schema v10）。

千问办公没有公开的模型 API，也没有可以长期保存的厂商 Key：它用 OAuth 登录，再按需换取 2 小时有效的 MaaS 临时票。本插件复现了这套链路，并把 14 个模型以 OpenAI 兼容的形式提供给 CPA。

> 作者：王小帅 · 许可：MIT · 平台：目前仅 Windows（本机登录态导入依赖 DPAPI）

---

## 功能

| 能力 | 说明 |
|---|---|
| `model_provider` | 14 个千问办公原生模型，默认 `qwenwork/` 前缀，避免与内置渠道重名 |
| `auth_provider` | OAuth 设备码登录（PKCE）+ 本机客户端登录态导入，支持多账号 |
| `executor` | `chat-completions` 协议；SSE 流式、工具调用、图片输入 |
| `quota_provider` | 套餐与额度查询；总额度使用官方用量百分比，月度/长期/每日显示余额 |
| `management_api` | 插件卡片「千问办公」入口，内嵌账号、额度、模型面板 |

Response / Anthropic Messages 协议由 CPA 内核转换，插件只实现 `chat-completions`。

## 原理

```
CPA 请求
  └─ 插件 executor
       ├─ 取凭据（OAuth 文件 / 本机客户端登录态）
       ├─ POST /api/desktop-sessions   → 2 小时 sk-mm 票（缓存）
       └─ POST {base_url}/v1/chat/completions
            └─ SSE 原样回传 → host.stream.emit
```

千问办公的模型调用本身就是 OpenAI 兼容格式（`/v1/chat/completions`、`/v1/responses`、`/v1/messages` 都原生支持），插件只做鉴权、换票和转发。

## 安装

1. 从 [Releases](https://github.com/wangxiaoshuai136/cpa-plugin-qwork-oauth/releases) 下载 `qwenwork_<版本>_windows_amd64.zip`，解压得到 `qwenwork.dll`；
2. 放到 `<CPA 数据目录>/plugins/windows/amd64/qwenwork-v<版本>.dll`；
3. 在 `config.yaml` 启用：

```yaml
plugins:
  enabled: true
  configs:
    qwenwork:
      enabled: true
```

4. 重启核心，或在「插件」页停用再启用一次以热加载。

## 配置

| 字段 | 默认 | 说明 |
|---|---|---|
| `auth_dir` | `%APPDATA%\QwenWorkCN` | 千问办公客户端登录态目录（含 `auth-v2.dat` 与 `Local State`） |
| `model_prefix` | `qwenwork` | 模型名前缀；设为空字符串则使用原始 ID（可能被内置渠道去重） |
| `base_url` | 空 | 覆盖换票返回的 MaaS 地址，一般不用填 |
| `public_base_url` | `http://127.0.0.1:8317` | CPA 核心地址，仅用于生成插件图标 URL |

## 登录与授权

### 方式一：从 CPA 登录（推荐，多账号）

1. 插件卡片 →「账号授权」→「添加账号」；
2. 浏览器打开千问办公授权页并登录；
3. 登录完成后浏览器会提示打开 `qwenwork-cn://`，**可以直接取消**，这不影响取票；
4. CPA 会自动轮询并生成 `oauth/qwenwork-<uid>.json`。

每个账号一份凭证，可在「凭证文件」页停用或删除。

### 方式二：导入本机客户端登录态

本机已登录千问办公时，新建 `oauth/qwenwork-local.json`：

```json
{
  "type": "qwenwork",
  "provider": "qwenwork",
  "oauth_provider": "qwenwork",
  "auth_kind": "local",
  "account_id": "qwenwork-local",
  "disabled": false,
  "auth_dir": "C:\\Users\\<用户>\\AppData\\Roaming\\QwenWorkCN",
  "name": "千问办公（本机登录）",
  "display_name": "本机登录",
  "key_source": "local"
}
```

插件每次请求读取客户端登录态（DPAPI 解密），不复制、不轮换 refresh token。客户端退出登录后该凭证失效。

## 模型

`qwenwork/flash`、`qwenwork/pro`、`qwenwork/qwen4-max`、`qwenwork/qwen3.8-max-preview`、`qwenwork/qwen3.8-flash`、`qwenwork/minimax-m3.1-flash`、`qwenwork/glm-5.4`、`qwenwork/ling-3.1-flash`、`qwenwork/deepseek-flash`、`qwenwork/deepseek-v4-pro`、`qwenwork/glm-5.3`、`qwenwork/glm-5.3-flash`、`qwenwork/kimi-k3.1-pro`、`qwenwork/step-5-preview`

模型页会显示上下文长度、输入模态（text / image）与是否推理模型。

## 额度

- 总额度：`GET /api/v2/quota/usage` 的 `total_usage_percentage`，进度条为真实剩余比例；
- 月度 / 长期 / 每日：来自 `/api/v1/adapter/user/wallets` 的余额；
- 个人免费版上游不提供月度/长期上限，因此这几项只显示余额，不伪造百分比。

## 构建

需要 Go 1.27+ 与 mingw64 gcc：

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags "-s -w" -o bin/qwenwork.dll ./cmd/qwenwork-plugin
```

推送 `v*` 标签会触发 Release 工作流，自动产出
`qwenwork_<version>_windows_amd64.zip` 与 `checksums.txt`。

## 已知限制

- 上游按模型限流 **4 次/分钟**（免费账号），批量调用需退避；
- MaaS 票 2 小时过期，插件自动换票；401/403 自动重试一次；
- 本机导入模式依赖客户端保持登录；
- 仅 Windows 构建：OAuth 模式本身跨平台，但当前代码的 DPAPI 部分尚未拆分；
- 非官方接口，服务端改版可能导致失效。

## 安全

- 插件只读取本机登录态，不上传凭据；
- 凭证保存在 CPA 的 `oauth/` 目录，与其他 provider 一致；
- 调用方仍是本机 CPA，请勿把 8317 暴露到公网。

## 文档

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)：插件 ABI、鉴权与请求流程、额度口径
- [CHANGELOG.md](CHANGELOG.md)：版本记录

## 许可

MIT © 2026 王小帅



