package cpa

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"
)

func resolveOAuthDir() string {
	for _, c := range []string{"../oauth", "../../oauth", "oauth"} {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "../oauth"
}

func (p *Plugin) panelCredential() (*credential, *credentialFile, error) {
	cfg := p.Config()
	dir := resolveOAuthDir()
	files, _ := filepath.Glob(filepath.Join(dir, "qwenwork-*.json"))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc credentialFile
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue
		}
		if doc.Disabled {
			continue
		}
		cred, err := credentialFromStorage(raw, cfg)
		if err == nil {
			return cred, &doc, nil
		}
	}
	cred, err := credentialFromStorage(nil, cfg)
	if err != nil {
		return nil, nil, err
	}
	return cred, &credentialFile{AuthKind: "local", AuthDir: cfg.AuthDir}, nil
}

func (p *Plugin) handleManagementRegister(raw []byte) (any, error) {
	return pluginapi.ManagementRegistrationResponse{
		Resources: []pluginapi.ResourceRoute{
			{Path: "/panel", Menu: "千问办公", Description: "千问办公账号、额度与模型面板"},
			{Path: "/icon.png", Description: "千问办公图标"},
		},
	}, nil
}

func (p *Plugin) handleManagementHandle(raw []byte) (any, error) {
	var req pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	path := strings.ToLower(req.Path)
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if strings.HasSuffix(path, "/icon.png") {
		h := http.Header{}
		h.Set("Content-Type", "image/png")
		h.Set("Cache-Control", "no-cache")
		return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: iconBytes()}, nil
	}
	htmlBody := p.renderPanel()
	headers := http.Header{}
	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: headers, Body: []byte(htmlBody)}, nil
}

func (p *Plugin) renderPanel() string {
	cred, doc, err := p.panelCredential()
	planName, remaining, monthly, longterm, daily := "—", 0.0, 0.0, 0.0, 0.0
	walletRows := ""
	authKind := "本机登录"
	accountName := "—"
	accountID := "—"
	if doc != nil {
		if doc.AuthKind == "oauth" {
			authKind = "OAuth"
		}
		accountName = firstNonEmpty(doc.DisplayName, doc.Label, "—")
		accountID = doc.AccountID
	}
	if err == nil && cred != nil {
		if acct, w, e := fetchQuotaData(p, cred); e == nil {
			planName = acct.Data.Plan.Name
			if planName == "" {
				planName = "千问办公"
			}
			remaining = acct.Data.Quota.Remaining
			monthly = w.Data.MonthlyCredits.TotalBalance
			longterm = w.Data.LongtermCredits.TotalBalance
			daily = w.Data.DailyCredits.TotalBalance
			for _, x := range w.Data.ActiveWallets.Wallets {
				walletRows += fmt.Sprintf(`<tr><td>%.2f</td><td>%s</td></tr>`, x.Balance, html.EscapeString(x.ValidTo))
			}
		}
	}
	prefix := strings.Trim(strings.TrimSpace(p.Config().ModelPrefix), "/")
	models := StaticModels(prefix)
	var modelRows strings.Builder
	for _, m := range models {
		mods := strings.Join(m.SupportedInputModalities, "/")
		modelRows.WriteString(fmt.Sprintf(`<tr><td><b>%s</b><div class="sub">%s</div></td><td><code>%s</code></td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(m.DisplayName), html.EscapeString(m.Description), html.EscapeString(m.ID), html.EscapeString(mods), html.EscapeString(fmt.Sprint(m.ContextLength))))
	}
	if walletRows == "" {
		walletRows = `<tr><td colspan="2" class="muted">暂无钱包明细</td></tr>`
	}
	return strings.NewReplacer(
		"{{plan}}", html.EscapeString(planName),
		"{{remaining}}", fmt.Sprintf("%.2f", remaining),
		"{{monthly}}", fmt.Sprintf("%.2f", monthly),
		"{{longterm}}", fmt.Sprintf("%.2f", longterm),
		"{{daily}}", fmt.Sprintf("%.2f", daily),
		"{{account}}", html.EscapeString(accountName),
		"{{accountID}}", html.EscapeString(accountID),
		"{{authKind}}", html.EscapeString(authKind),
		"{{modelCount}}", fmt.Sprint(len(models)),
		"{{wallets}}", walletRows,
		"{{models}}", modelRows.String(),
	).Replace(panelHTML)
}

const panelHTML = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>千问办公 · 面板</title>
<style>
:root{--bg:#0b0f16;--card:#121826;--line:#243047;--text:#e8eefc;--muted:#8b96ab;--accent:#7c8cff}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(900px 400px at 20% -10%,rgba(124,140,255,.18),transparent 60%),var(--bg);color:var(--text);font:14px/1.55 "Segoe UI","Microsoft YaHei",system-ui}
.wrap{max-width:980px;margin:0 auto;padding:26px 18px 50px}
.head{display:flex;justify-content:space-between;align-items:center;margin-bottom:18px}
h1{font-size:20px;margin:0}.muted,.sub{color:var(--muted)}.sub{font-size:11.5px}
button{background:#1a2335;color:var(--text);border:1px solid var(--line);border-radius:10px;padding:7px 13px;cursor:pointer}
.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:14px}
.card{background:linear-gradient(180deg,#141b2b,#0f1420);border:1px solid var(--line);border-radius:14px;padding:14px 16px;margin-bottom:14px}
.label{font-size:12px;color:var(--muted)}.value{font-size:24px;font-weight:650;margin-top:4px}
table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:8px 10px;border-bottom:1px solid #1d2638;font-size:12.5px}
th{color:var(--muted);font-weight:600}tr:last-child td{border-bottom:0}
code{background:#0b1018;border:1px solid #1e283b;border-radius:6px;padding:1px 6px;font-size:11.5px}
@media(max-width:760px){.cards{grid-template-columns:1fr 1fr}}
</style></head><body><div class="wrap">
<div class="head"><h1>千问办公</h1><button onclick="location.reload()">刷新</button></div>
<div class="cards">
<div class="card"><div class="label">剩余额度</div><div class="value">{{remaining}}</div></div>
<div class="card"><div class="label">月度额度</div><div class="value">{{monthly}}</div></div>
<div class="card"><div class="label">长期额度</div><div class="value">{{longterm}}</div></div>
<div class="card"><div class="label">每日额度</div><div class="value">{{daily}}</div></div>
</div>
<div class="card"><div class="label">当前账号</div>
<div style="margin-top:6px"><b>{{account}}</b> <span class="sub">{{accountID}}</span></div>
<div class="sub" style="margin-top:4px">套餐：{{plan}} · 认证：{{authKind}}</div></div>
<div class="card"><div class="label">钱包</div><table><thead><tr><th>余额</th><th>到期</th></tr></thead><tbody>{{wallets}}</tbody></table></div>
<div class="card"><div class="label">模型（{{modelCount}}）</div><table><thead><tr><th>模型</th><th>ID</th><th>输入</th><th>上下文</th></tr></thead><tbody>{{models}}</tbody></table></div>
</div></body></html>`

