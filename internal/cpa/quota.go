package cpa

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"
)

type accountContext struct {
	Data struct {
		Plan struct {
			Name       string `json:"name"`
			IsPersonal bool   `json:"is_personal_version"`
		} `json:"plan"`
		Quota struct {
			Remaining float64 `json:"remaining"`
			Exceeded  bool    `json:"exceeded"`
			Total     float64 `json:"total"`
			Used      float64 `json:"used"`
		} `json:"quota"`
	} `json:"data"`
}

type walletInfo struct {
	Balance float64 `json:"balance"`
	ValidTo string  `json:"valid_to"`
}

type walletsResponse struct {
	Data struct {
		ActiveWallets struct {
			Wallets []walletInfo `json:"wallets"`
		} `json:"active_wallets"`
		DailyCredits struct {
			TotalBalance float64 `json:"total_balance"`
		} `json:"daily_credits"`
		LongtermCredits struct {
			TotalBalance float64 `json:"total_balance"`
		} `json:"longterm_credits"`
		MonthlyCredits struct {
			TotalBalance float64 `json:"total_balance"`
		} `json:"monthly_credits"`
	} `json:"data"`
}

func fetchJSON(p *Plugin, cred *credential, url string, out any) error {
	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "Bearer "+cred.token)
	req.Header.Set("accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return json.Unmarshal(body, out)
}

type quotaUsage struct {
	UserType             string  `json:"user_type"`
	TotalUsagePercentage float64 `json:"total_usage_percentage"`
	IsQuotaExceeded      bool    `json:"is_quota_exceeded"`
}

func fetchQuotaData(p *Plugin, cred *credential) (accountContext, walletsResponse, error) {
	var acct accountContext
	var wallets walletsResponse
	if err := fetchJSON(p, cred, cred.origin+"/api/v1/adapter/user/account-context?include=user,plan,quota", &acct); err != nil {
		return acct, wallets, fmt.Errorf("查询账户失败: %w", err)
	}
	_ = fetchJSON(p, cred, cred.origin+"/api/v1/adapter/user/wallets", &wallets)
	return acct, wallets, nil
}

func fetchUsagePercentage(p *Plugin, cred *credential) (float64, bool) {
	var u quotaUsage
	if err := fetchJSON(p, cred, cred.origin+"/api/v2/quota/usage", &u); err != nil {
		return 0, false
	}
	return u.TotalUsagePercentage, true
}

func (p *Plugin) handleQuotaIdentifier() (any, error) {
	return rpcIdentifierResponse{Identifier: ProviderID, Identifiers: []string{ProviderID}, CredentialProviders: []string{ProviderID}}, nil
}

func (p *Plugin) handleQuotaDescribe() (any, error) {
	return pluginapi.QuotaDescribeResponse{
		SupportedProviders: []string{ProviderID},
		DisplayName:        "千问办公额度",
		SupportsReset:      false,
	}, nil
}

func (p *Plugin) handleQuotaFetch(raw []byte) (any, error) {
	var req pluginapi.QuotaFetchRequest
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	cred, err := credentialFromStorage(req.StorageJSON, p.Config())
	if err != nil {
		return nil, fmt.Errorf("凭据不可用: %w", err)
	}
	acct, wallets, err := fetchQuotaData(p, cred)
	if err != nil {
		return nil, err
	}
	planName := acct.Data.Plan.Name
	if planName == "" {
		planName = "千问办公"
	}
	usedPct, hasPct := fetchUsagePercentage(p, cred)
	remainingFrac := 0.0
	desc := fmt.Sprintf("剩余 %.2f", acct.Data.Quota.Remaining)
	if hasPct {
		remainingFrac = 1 - usedPct/100
		if remainingFrac < 0 {
			remainingFrac = 0
		}
		desc = fmt.Sprintf("剩余 %.2f · 已用 %.2f%%", acct.Data.Quota.Remaining, usedPct)
	} else {
		b := loadBaseline()
		peak := peakOf(b, req.AuthID+":total", acct.Data.Quota.Remaining)
		remainingFrac = fraction(acct.Data.Quota.Remaining, peak)
		raw, _ := json.MarshalIndent(b, "", "  ")
		_ = os.WriteFile(baselinePath(), raw, 0o644)
		desc = fmt.Sprintf("剩余 %.2f（按峰值估算）", acct.Data.Quota.Remaining)
	}
	groups := []pluginapi.QuotaGroup{{
		DisplayName: "总额度",
		Buckets: []pluginapi.QuotaBucket{{
			Window:            "总额度",
			RemainingFraction: math.Round(remainingFrac*10000) / 10000,
			Description:       desc,
		}},
	}}
	summary := []pluginapi.QuotaMetric{
		{Key: "monthly", Label: "月度额度", Value: math.Round(wallets.Data.MonthlyCredits.TotalBalance*100) / 100, Format: "number"},
		{Key: "longterm", Label: "长期额度", Value: math.Round(wallets.Data.LongtermCredits.TotalBalance*100) / 100, Format: "number"},
		{Key: "daily", Label: "每日额度", Value: math.Round(wallets.Data.DailyCredits.TotalBalance*100) / 100, Format: "number"},
	}
	return pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{Plan: planName, TierName: planName},
		Summary:      summary,
		Groups:       groups,
	}, nil
}

type baselineFile struct {
	Peaks map[string]float64 `json:"peaks"`
}

func baselinePath() string {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "cpa-plugin-qwenwork")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "quota-baseline.json")
}

func loadBaseline() baselineFile {
	b := baselineFile{Peaks: map[string]float64{}}
	if raw, err := os.ReadFile(baselinePath()); err == nil {
		_ = json.Unmarshal(raw, &b)
	}
	if b.Peaks == nil {
		b.Peaks = map[string]float64{}
	}
	return b
}

func peakOf(b baselineFile, key string, value float64) float64 {
	p := b.Peaks[key]
	if value > p {
		p = value
	}
	b.Peaks[key] = p
	return p
}

func fraction(value, peak float64) float64 {
	if peak <= 0 {
		return 0
	}
	f := value / peak
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return math.Round(f*10000) / 10000
}

func buildQuotaGroups(p *Plugin, authID string, remaining float64, wallets walletsResponse) []pluginapi.QuotaGroup {
	b := loadBaseline()
	defer func() {
		raw, _ := json.MarshalIndent(b, "", "  ")
		_ = os.WriteFile(baselinePath(), raw, 0o644)
	}()
	type tier struct {
		name  string
		value float64
	}
	tiers := []tier{
		{"总额度", remaining},
		{"月度", wallets.Data.MonthlyCredits.TotalBalance},
		{"长期", wallets.Data.LongtermCredits.TotalBalance},
	}
	if wallets.Data.DailyCredits.TotalBalance > 0 {
		tiers = append(tiers, tier{"每日", wallets.Data.DailyCredits.TotalBalance})
	}
	buckets := make([]pluginapi.QuotaBucket, 0, len(tiers))
	for _, t := range tiers {
		key := authID + ":" + t.name
		peak := peakOf(b, key, t.value)
		buckets = append(buckets, pluginapi.QuotaBucket{
			Window:            t.name,
			RemainingFraction: fraction(t.value, peak),
			Description:       fmt.Sprintf("剩余 %.2f", t.value),
		})
	}
	return []pluginapi.QuotaGroup{{DisplayName: "额度档位", Buckets: buckets}}
}


