## 0.1.1 - 2026-11-27

### 修复

- 账号授权不再自行拉起浏览器，统一由 CPA 按其浏览器设置打开（默认浏览器 / 指定浏览器 / 不自动打开）
- 插件显示名改为 `QWork OAuth`，OAuth 条目名为 `QWork`，OAuth 页不再出现 `QWork OAuth OAuth`
- 版本号升到 0.1.1，确保内核重新注册元数据
# Changelog

## 0.1.0 - 2026-11-27

首个公开版本。

### 新增

- 原生 CPA 插件（C ABI v1 / Schema v10），provider id `qwenwork`
- 14 个千问办公模型，默认 `qwenwork/` 前缀
- OAuth 设备码登录（PKCE）+ 本机登录态导入，支持多账号
- `chat-completions` 执行器：SSE 流式、工具调用、图片输入
- 额度查询：官方用量百分比 + 月度/长期/每日余额
- 插件卡片管理面板与圆角图标

### 修复

- 流式返回改为 `host.stream.emit`，修复 `unsupported type: <-chan` 500
- 凭证强制 `.json` 文件名，修复重复卡片与删除 404
- 上游固定 `Accept-Encoding: identity`，修复 gzip 头错配导致的 `terminated`

