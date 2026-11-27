<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="QWork OAuth logo">
</p>

<h1 align="center">QWork OAuth</h1>

<p align="center">
  把 <b>千问办公（QwenWork）</b> 接入 <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI</a> 的原生插件
</p>

<p align="center">
  <a href="https://github.com/wangxiaoshuai136/cpa-plugin-qwork-oauth/releases"><img src="https://img.shields.io/github/v/release/wangxiaoshuai136/cpa-plugin-qwork-oauth?label=release" alt="Release"></a>
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-0078d4" alt="macOS, Linux, Windows">
  <img src="https://img.shields.io/badge/CLIProxyAPI-plugin-4f46e5" alt="CLIProxyAPI plugin">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  <a href="CHANGELOG.md">更新日志</a> ·
  <a href="docs/ARCHITECTURE.md">架构说明</a> ·
  <a href="https://github.com/router-for-me/CLIProxyAPI-Plugins-Store/pull/405">插件库 PR</a>
</p>

千问办公没有公开的模型 API，也没有可以长期保存的厂商 Key：它用 OAuth 登录，再按需换取 2 小时有效的 MaaS 临时票。QWork OAuth 复现了这套链路，把 14 个模型以 OpenAI 兼容的形式接进 CPA，并提供额度查询和账号面板。

> [!WARNING]
> 本项目仅供学习与技术交流。请使用你自己有权使用的账号，遵守千问办公的服务条款。项目与阿里、通义千问无隶属关系，使用后果由使用者自行承担。

## 特性

| 能力 | 说明 |
| --- | --- |
| `model_provider` | 14 个原生模型，默认 `qwenwork/` 前缀，避免与内置渠道重名 |
| `auth_provider` | OAuth 设备码登录（PKCE）+ 本机客户端登录态导入，支持多账号 |
| `executor` | `chat-completions`：SSE 流式、工具调用、图片输入 |
| `quota_provider` | 套餐与额度：真实用量百分比 + 月度/长期/每日余额 |

Responses 与 Anthropic Messages 由 CPA 内核转换，插件只实现 `chat-completions`。

## 工作原理

```
CPA 请求
  └─ executor
       ├─ 取凭据（OAuth 文件 / 本机客户端登录态）
       ├─ POST /api/desktop-sessions   → 2 小时 sk-mm 票（按 token 缓存）
       └─ POST {base_url}/v1/chat/completions
            └─ SSE 原样回传 → host.stream.emit
```

千问办公的模型接口本身就是 OpenAI 兼容格式（`/v1/chat/completions`、`/v1/responses`、`/v1/messages`），插件负责鉴权、换票与转发。

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

4. 重启核心，或在「插件」页停用再启用一次热加载。

## 配置

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `auth_dir` | `%APPDATA%\QwenWorkCN` | 千问办公客户端登录态目录 |
| `model_prefix` | `qwenwork` | 模型名前缀；留空则用原始 ID |
| `base_url` | 空 | 覆盖换票返回的 MaaS 地址，一般不用填 |
| `public_base_url` | `http://127.0.0.1:8317` | CPA 核心地址，用于生成面板/图标链接 |

## 登录与授权

### 从 CPA 登录（推荐，多账号）

1. 插件卡片 →「账号授权」→「添加账号」；
2. 浏览器打开授权页，选择账号完成登录；
3. 浏览器最后会提示打开 `qwenwork-cn://`，**取消即可**，不影响取票；
4. CPA 自动轮询并在 `oauth/` 生成 `qwenwork-<uid>.json`。

浏览器由 CPA 按设置打开：默认浏览器 / 指定浏览器 / 不自动打开。每个账号一份凭证，可在「凭证文件」页停用或删除。

### 导入本机客户端登录态

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

插件每次请求实时读取客户端登录态（DPAPI 解密），不复制文件、不轮换 refresh token。客户端退出登录后该凭证失效。

## 模型

| 模型 | 说明 |
| --- | --- |
| `qwenwork/flash` | 标准，极速全能，支持视觉 |
| `qwenwork/pro` | 高级，性能均衡 |
| `qwenwork/qwen4-max` | 旗舰，支持视觉 |
| `qwenwork/qwen3.8-max-preview` | 全维旗舰 |
| `qwenwork/qwen3.8-flash` | 轻量高频 |
| `qwenwork/minimax-m3.1-flash` | 多模态办公 |
| `qwenwork/glm-5.4` | 编程与多步骤 |
| `qwenwork/ling-3.1-flash` | 长程推理 |
| `qwenwork/deepseek-flash` | 视觉推理 |
| `qwenwork/deepseek-v4-pro` | 深度推理 |
| `qwenwork/glm-5.3` | 百万上下文 |
| `qwenwork/glm-5.3-flash` | 轻量多模态 |
| `qwenwork/kimi-k3.1-pro` | 长上下文 |
| `qwenwork/step-5-preview` | 多模态 CLI |

## 额度

- **总额度**：`GET /api/v2/quota/usage` 的 `total_usage_percentage`，进度条为真实剩余比例；
- **月度 / 长期 / 每日**：来自 `/api/v1/adapter/user/wallets` 的余额；
- 个人免费版上游不提供月度/长期上限，因此这几项只显示余额，不伪造百分比。

## 额度展示

额度直接显示在「凭证文件」的账号卡片上；插件不再注册动态 resource 页面（`/v0/resource/plugins/` 仅提供静态图标）。

## 构建

需要 Go 1.27+ 与 mingw64 gcc：

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags "-s -w" -o bin/qwenwork.dll ./cmd/qwenwork-plugin
```

推送 `v*` 标签会触发 Release 工作流，产出 `qwenwork_<version>_windows_amd64.zip` 与 `checksums.txt`。

## 已知限制

- 上游按模型限流 **4 次/分钟**（免费账号），批量调用需退避；
- MaaS 票 2 小时过期，插件自动换票；401/403 自动重试一次；
- 本机导入模式仅 Windows（DPAPI），其他平台请使用 OAuth 登录；
- 发布产物覆盖 macOS（amd64/arm64）、Linux（amd64/arm64）、Windows（amd64/arm64）；
- 非官方接口，服务端改版可能导致失效。

## 安全

- 插件只读取本机登录态，不上传凭据；
- 凭证保存在 CPA 的 `oauth/` 目录，与其他 provider 一致；
- 请勿把 CPA 的 8317 端口暴露到公网。

## 许可

MIT © 2026 王小帅

