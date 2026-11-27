package cpa

import (
	"encoding/json"
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
		Routes: []pluginapi.ManagementRoute{
			{Method: http.MethodGet, Path: "/v0/management/qwenwork/api/state", Description: "QWork account, quota and model state"},
		},
		Resources: []pluginapi.ResourceRoute{
			{Path: "/panel", Menu: "千问办公", Description: "千问办公账号与额度面板（静态页面，通过管理鉴权读取数据）"},
			{Path: "/icon.png", Description: "QWork icon"},
		},
	}, nil
}

func (p *Plugin) handleManagementHandle(raw []byte) (any, error) {
	var req pluginapi.ManagementRequest
	_ = json.Unmarshal(raw, &req)
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
	if strings.Contains(path, "/api/state") {
		return p.handlePanelState()
	}
	h := http.Header{}
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: []byte(panelHTML)}, nil
}

type panelState struct {
	Account struct {
		Name string `json:"name"`
		ID   string `json:"id"`
		Kind string `json:"kind"`
	} `json:"account"`
	Plan    string  `json:"plan"`
	Quota   float64 `json:"quota"`
	UsedPct float64 `json:"usedPct"`
	Monthly float64 `json:"monthly"`
	Long    float64 `json:"longterm"`
	Daily   float64 `json:"daily"`
	Models  int     `json:"models"`
	Error   string  `json:"error,omitempty"`
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func (p *Plugin) handlePanelState() (any, error) {
	var st panelState
	cred, doc, err := p.panelCredential()
	if err != nil {
		st.Error = "凭据不可用: " + err.Error()
	} else {
		if doc != nil {
			st.Account.Name = firstNonEmpty(doc.DisplayName, doc.Label, "—")
			st.Account.ID = doc.AccountID
			st.Account.Kind = doc.AuthKind
			if st.Account.Kind == "" {
				st.Account.Kind = "local"
			}
		}
		acct, wallets, e := fetchQuotaData(p, cred)
		if e != nil {
			st.Error = e.Error()
		} else {
			st.Plan = acct.Data.Plan.Name
			st.Quota = round2(acct.Data.Quota.Remaining)
			st.Monthly = round2(wallets.Data.MonthlyCredits.TotalBalance)
			st.Long = round2(wallets.Data.LongtermCredits.TotalBalance)
			st.Daily = round2(wallets.Data.DailyCredits.TotalBalance)
			if pct, ok := fetchUsagePercentage(p, cred); ok {
				st.UsedPct = round2(pct)
			}
		}
	}
	st.Models = len(StaticModels(strings.Trim(strings.TrimSpace(p.Config().ModelPrefix), "/")))
	body, _ := json.Marshal(st)
	h := http.Header{}
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: body}, nil
}

const panelHTML = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>QWork OAuth</title>
<style>
:root{--bg:#0b0f16;--card:#121826;--line:#243047;--text:#e8eefc;--muted:#8b96ab;--accent:#34d399}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(900px 400px at 20% -10%,rgba(52,211,153,.16),transparent 60%),var(--bg);color:var(--text);font:14px/1.55 "Segoe UI","Microsoft YaHei",system-ui}
.wrap{max-width:760px;margin:0 auto;padding:26px 18px 50px}
h1{font-size:20px;margin:0 0 16px}
.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:14px}
.card{background:linear-gradient(180deg,#141b2b,#0f1420);border:1px solid var(--line);border-radius:14px;padding:14px 16px}
.label{font-size:12px;color:var(--muted)}.value{font-size:22px;font-weight:650;margin-top:4px}
.muted{color:var(--muted)}input{background:#0d121b;border:1px solid var(--line);color:var(--text);border-radius:9px;padding:8px 10px;width:100%}
button{background:#1a2335;color:var(--text);border:1px solid var(--line);border-radius:9px;padding:8px 14px;cursor:pointer}
@media(max-width:720px){.cards{grid-template-columns:1fr 1fr}}
</style></head><body><div class="wrap">
<h1>千问办公</h1>
<div id="auth" class="card" style="display:none;margin-bottom:14px">
  <div class="label">管理密钥</div>
  <div style="display:flex;gap:8px;margin-top:8px"><input id="key" placeholder="粘贴 CPA 管理密钥"><button onclick="saveKey()">确定</button></div>
  <p class="muted" style="font-size:12px">数据通过 /v0/management/qwenwork/api/state 读取，需要管理密钥。</p>
</div>
<div class="cards">
  <div class="card"><div class="label">剩余额度</div><div class="value" id="quota">—</div></div>
  <div class="card"><div class="label">月度额度</div><div class="value" id="monthly">—</div></div>
  <div class="card"><div class="label">长期额度</div><div class="value" id="long">—</div></div>
  <div class="card"><div class="label">每日额度</div><div class="value" id="daily">—</div></div>
</div>
<div class="card"><div class="label">账号</div><div id="account" style="margin-top:6px">—</div><div class="muted" id="meta" style="font-size:12px"></div></div>
<div class="card" style="margin-top:14px"><div class="label">状态</div><div id="status" class="muted" style="margin-top:6px">加载中…</div></div>
</div>
<script>
const KEY_NAMES=['cli-proxy-auth','managementKey','management_key','management-key'];
function readStoredKey(){
  for(const n of KEY_NAMES){
    const v=localStorage.getItem(n)||sessionStorage.getItem(n);
    if(!v) continue;
    try{const o=JSON.parse(v);for(const k of ['managementKey','management_key','key','token','secret']) if(o&&o[k]) return String(o[k]);}catch(e){}
    if(v.length<200) return v;
  }
  return '';
}
function saveKey(){const v=document.getElementById('key').value.trim(); if(v){sessionStorage.setItem('managementKey',v); load();}}
async function load(){
  let key=readStoredKey();
  if(!key){document.getElementById('auth').style.display='block';document.getElementById('status').textContent='需要管理密钥';return;}
  try{
    const r=await fetch('/v0/management/qwenwork/api/state',{headers:{Authorization:'Bearer '+key}});
    if(r.status===401){document.getElementById('auth').style.display='block';document.getElementById('status').textContent='管理密钥无效';return;}
    const d=await r.json();
    document.getElementById('quota').textContent=d.quota??'—';
    document.getElementById('monthly').textContent=d.monthly??'—';
    document.getElementById('long').textContent=d.longterm??'—';
    document.getElementById('daily').textContent=d.daily??'—';
    document.getElementById('account').textContent=(d.account&&d.account.name)||'—';
    document.getElementById('meta').textContent='套餐 '+(d.plan||'—')+' · 认证 '+(d.account&&d.account.kind||'—')+' · 已用 '+(d.usedPct??0)+'% · '+(d.models||0)+' 个模型';
    document.getElementById('status').textContent=d.error?('错误：'+d.error):'正常';
  }catch(e){document.getElementById('status').textContent='请求失败：'+e;}
}
load();
</script></body></html>`
