## 0.2.1 - 2026-11-27

### 修复

- 按审计要求恢复面板：`/v0/resource/plugins/` 只提供静态 HTML 与图标，账号/额度数据改由鉴权接口 `/v0/management/qwenwork/api/state` 提供
- 面板页面复用同源管理密钥（`cli-proxy-auth` / `managementKey`），未登录时提示手动输入
## 0.2.0 - 2026-11-27

### 变更

- 发布全平台产物：macOS / Linux / Windows 的 amd64 与 arm64
- 本机登录态导入仅在 Windows 可用，其他平台使用 OAuth 登录
- 移除 `/v0/resource/plugins/` 下的动态面板，只保留静态图标；额度仍在凭证卡片展示
- 版本升到 0.2.0，满足插件库审计要求
## 0.1.3 - 2026-11-27

### 修复

- `auth.parse` / `auth.refresh` 回传 `note`，凭证卡片上的账号备注现在会正确保存并显示
- 凭证文件结构保留 `note` 字段，刷新时不再丢失
## 0.1.2 - 2026-11-27

### 优化

- 图标改用官网 SVG 矢量源重新栅格化，输出 128×128 与 512×512 圆角图标
- README 增加高清 logo、徽章与完整使用说明
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





