package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetResponseBodySizeLimit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		size      int
		wantError bool
	}{
		{name: "accepts response at limit", size: maxAdvancedCustomBalanceResponseBytes},
		{name: "rejects oversized response", size: maxAdvancedCustomBalanceResponseBytes + 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := strings.Repeat("x", tc.size)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			body, err := GetResponseBody(http.MethodGet, server.URL, &model.Channel{}, nil)
			if tc.wantError {
				require.ErrorContains(t, err, "balance response exceeds")
				assert.Nil(t, body)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, payload, string(body))
		})
	}
}

func TestGetDeepSeekBalanceUSD(t *testing.T) {
	tests := []struct {
		name            string
		responseJSON    string
		usdExchangeRate float64
		want            float64
		wantErrContains string
	}{
		{
			name:            "prefers USD when USD precedes CNY",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"12.50"},{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			want:            12.5,
		},
		{
			name:            "prefers USD when CNY precedes USD",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"},{"currency":"USD","total_balance":"12.50"}]}`,
			usdExchangeRate: 7.3,
			want:            12.5,
		},
		{
			name:            "converts CNY when USD is absent",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			want:            10,
		},
		{
			name:            "converts CNY when USD balance is zero",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"0.00"},{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			want:            10,
		},
		{
			name:            "converts CNY when zero USD follows CNY",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"},{"currency":"USD","total_balance":"0.00"}]}`,
			usdExchangeRate: 7.3,
			want:            10,
		},
		{
			name:            "returns zero when USD balance is zero and CNY is absent",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"0.00"}]}`,
			usdExchangeRate: 7.3,
			want:            0,
		},
		{
			name:            "returns error when USD and CNY are absent",
			responseJSON:    `{"balance_infos":[{"currency":"EUR","total_balance":"10.00"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "currency USD or CNY not found",
		},
		{
			name:            "returns USD parse error instead of falling back to CNY",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"invalid"},{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "invalid syntax",
		},
		{
			name:            "rejects NaN USD balance",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"NaN"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "USD balance must be finite",
		},
		{
			name:            "rejects negative USD balance",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"-1.00"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "USD balance must be non-negative",
		},
		{
			name:            "rejects positive infinity CNY balance",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"+Inf"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "CNY balance must be finite",
		},
		{
			name:            "rejects negative CNY balance",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"-7.30"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "CNY balance must be non-negative",
		},
		{
			name:            "returns error for non-positive CNY exchange rate",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 0,
			wantErrContains: "USD exchange rate must be greater than zero",
		},
		{
			name:            "rejects NaN CNY exchange rate",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: math.NaN(),
			wantErrContains: "USD exchange rate must be finite",
		},
		{
			name:            "rejects positive infinity CNY exchange rate",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: math.Inf(1),
			wantErrContains: "USD exchange rate must be finite",
		},
		{
			name:            "rejects CNY conversion overflow",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"1.7976931348623157e+308"}]}`,
			usdExchangeRate: math.SmallestNonzeroFloat64,
			wantErrContains: "converted USD balance must be finite",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var response DeepSeekUsageResponse
			require.NoError(t, common.Unmarshal([]byte(test.responseJSON), &response))

			balance, err := getDeepSeekBalanceUSD(response, test.usdExchangeRate)
			if test.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.InDelta(t, test.want, balance, 1e-12)
		})
	}
}

func TestUpdateChannelNewAPIBalanceUsesTheSmallerRemainingQuota(t *testing.T) {
	for _, tc := range []struct {
		name    string
		usage   string
		want    float64
		wantErr string
	}{
		{name: "user quota below the key quota", usage: `{"code":true,"data":{"total_available":2000000,"unlimited_quota":false,"user_quota":{"total_available":500000}}}`, want: 0.5},
		{name: "key quota below the user quota", usage: `{"code":true,"data":{"total_available":500000,"unlimited_quota":false,"user_quota":{"total_available":2000000}}}`, want: 0.5},
		{name: "unlimited key uses the user quota", usage: `{"code":true,"data":{"total_available":0,"unlimited_quota":true,"user_quota":{"total_available":3000000}}}`, want: 3},
		{name: "key quota alone without user quota", usage: `{"code":true,"data":{"total_available":1500000,"unlimited_quota":false}}`, want: 1.5},
		{name: "unlimited key without user quota", usage: `{"code":true,"data":{"total_available":0,"unlimited_quota":true}}`, wantErr: "Allow querying user quota"},
		{name: "upstream rejects the key", usage: `{"success":false,"message":"invalid token"}`, wantErr: "invalid token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSub2APIBalanceTest(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/usage/token/":
					assert.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
					_, _ = w.Write([]byte(tc.usage))
				case "/api/status":
					assert.Empty(t, r.Header.Get("Authorization"), "the key is not sent to the public status endpoint")
					_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":1000000}}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: &baseURL, Key: "sk-upstream", Name: "new-api-upstream"}
			require.NoError(t, model.DB.Create(channel).Error)

			result, err := updateChannelBalance(channel)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tc.want, result.Balance, 1e-9)
			assert.InDelta(t, tc.want, channel.Balance, 1e-9)
		})
	}
}

func TestResolveUnifiedChannelExchangeRateAndMultiplier(t *testing.T) {
	origRates := operation_setting.GetGeneralSetting().CurrencyRates
	origUSD := operation_setting.USDExchangeRate
	origNTD := operation_setting.NTDExchangeRate
	t.Cleanup(func() {
		gs := operation_setting.GetGeneralSetting()
		gs.CurrencyRates = origRates
		operation_setting.USDExchangeRate = origUSD
		operation_setting.NTDExchangeRate = origNTD
	})

	gs := operation_setting.GetGeneralSetting()
	operation_setting.USDExchangeRate = 7.3
	operation_setting.NTDExchangeRate = 32.0
	gs.CurrencyRates = map[string]operation_setting.CurrencyRate{
		"USD": {Symbol: "$", Rate: 1.0},
		"CNY": {Symbol: "¥", Rate: 7.3},
		"TWD": {Symbol: "NT$", Rate: 32.0},
		"JPY": {Symbol: "¥", Rate: 150.0},
	}

	for _, tc := range []struct {
		name     string
		currency string
		rate     float64
		wantRate float64
		wantMult float64
	}{
		{name: "USD ignores legacy 35 default", currency: "USD", rate: 35.0, wantRate: 1.0, wantMult: 1.0},
		{name: "CNY ignores legacy 5 default", currency: "CNY", rate: 5.0, wantRate: 7.3, wantMult: 1.0 / 7.3},
		{name: "CNY preserves explicit USD-based override", currency: "CNY", rate: 7.1, wantRate: 7.1, wantMult: 1.0 / 7.1},
		{name: "TWD ignores legacy 1 default", currency: "TWD", rate: 1.0, wantRate: 32.0, wantMult: 1.0 / 32.0},
		{name: "NTD alias uses TWD rate", currency: "NTD", rate: 0, wantRate: 32.0, wantMult: 1.0 / 32.0},
		{name: "JPY uses unified rate table", currency: "JPY", rate: 0, wantRate: 150.0, wantMult: 1.0 / 150.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.InDelta(t, tc.wantRate, model.ResolveEffectiveChannelExchangeRate(tc.currency, tc.rate), 1e-9)
			assert.InDelta(t, tc.wantMult, model.ResolveUpstreamToQuotaUnitMultiplier(tc.currency, tc.rate), 1e-9)
		})
	}
}

func TestApplyAuthoritativeModelPricesConvertsUpstreamCurrencyToUSD(t *testing.T) {
	origDB := model.DB
	model.DB = nil
	origRates := operation_setting.GetGeneralSetting().CurrencyRates
	origUSD := operation_setting.USDExchangeRate
	origModelRatioBytes, err := common.Marshal(ratio_setting.GetModelRatioCopy())
	require.NoError(t, err)
	origCompletionRatioBytes, err := common.Marshal(ratio_setting.GetCompletionRatioCopy())
	require.NoError(t, err)
	origModelPriceBytes, err := common.Marshal(ratio_setting.GetModelPriceCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB = origDB
		gs := operation_setting.GetGeneralSetting()
		gs.CurrencyRates = origRates
		operation_setting.USDExchangeRate = origUSD
		_ = ratio_setting.UpdateModelRatioByJSONString(string(origModelRatioBytes))
		_ = ratio_setting.UpdateCompletionRatioByJSONString(string(origCompletionRatioBytes))
		_ = ratio_setting.UpdateModelPriceByJSONString(string(origModelPriceBytes))
	})

	gs := operation_setting.GetGeneralSetting()
	operation_setting.USDExchangeRate = 7.3
	gs.CurrencyRates = map[string]operation_setting.CurrencyRate{
		"USD": {Symbol: "$", Rate: 1.0},
		"CNY": {Symbol: "¥", Rate: 7.3},
		"TWD": {Symbol: "NT$", Rate: 32.0},
	}

	const cnyModel = "test-deepseek-cny-sync"
	const compareModel = "test-cny-vs-usd-priority"
	const discountedModel = "test-glm-5.3-discounted"
	const unconfiguredModel = "antigravity-preview-05-2026"

	// Channel 901: CNY upstream priced at ¥2/1M input, ¥8/1M output (1x), plus a 0.2x discounted glm-5.3 (¥1.6 / ¥5.6)
	// and an unconfigured 37.5 model.
	model.SetChannelUpstreamModelCosts(901, map[string]model.ChannelModelCostEntry{
		cnyModel: {
			ChannelID:       901,
			ChannelName:     "gc-default-1",
			Model:           cnyModel,
			QuotaType:       0,
			RawInputCost:    2.0,
			RawOutputCost:   8.0,
			InputCost:       2.0 / 7.3,
			OutputCost:      8.0 / 7.3,
			ModelRatio:      1.0,
			CompletionRatio: 4.0,
			BaseMultiplier:  1.0,
			Currency:        "CNY",
			ExchangeRate:    5.0, // legacy TWD-per-CNY default must fall back to 7.3
			FromUpstream:    true,
		},
		compareModel: {
			ChannelID:       901,
			ChannelName:     "gc-cny-1",
			Model:           compareModel,
			QuotaType:       0,
			RawInputCost:    2.0, // ¥2 CNY ≈ $0.274 USD
			RawOutputCost:   4.0,
			InputCost:       2.0 / 7.3,
			OutputCost:      4.0 / 7.3,
			ModelRatio:      1.0,
			CompletionRatio: 2.0,
			BaseMultiplier:  1.0,
			Currency:        "CNY",
			ExchangeRate:    7.3,
			FromUpstream:    true,
		},
		discountedModel: {
			ChannelID:       901,
			ChannelName:     "moyuu-go-0.2",
			Model:           discountedModel,
			QuotaType:       0,
			RawInputCost:    1.6, // 0.2x of ¥8
			RawOutputCost:   5.6, // 0.2x of ¥28
			InputCost:       1.6 / 7.3,
			OutputCost:      5.6 / 7.3,
			ModelRatio:      4.0,
			CompletionRatio: 3.5,
			BaseMultiplier:  0.2,
			Currency:        "CNY",
			ExchangeRate:    5.0,
			FromUpstream:    true,
		},
		unconfiguredModel: {
			ChannelID:       901,
			ChannelName:     "moyuu-go-0.2",
			Model:           unconfiguredModel,
			QuotaType:       0,
			ModelRatio:      37.5,
			CompletionRatio: 1.0,
			BaseMultiplier:  0.2,
			Currency:        "CNY",
			ExchangeRate:    7.3,
			FromUpstream:    true,
		},
	}, false)

	// Channel 902: USD upstream priced at $1/1M input, $2/1M output (higher USD price than ¥2 CNY),
	// plus a 0.4x discounted glm-5.3 (¥3.2 / ¥11.2 CNY).
	model.SetChannelUpstreamModelCosts(902, map[string]model.ChannelModelCostEntry{
		compareModel: {
			ChannelID:       902,
			ChannelName:     "official-usd",
			Model:           compareModel,
			QuotaType:       0,
			RawInputCost:    1.0, // $1.00 USD > ¥2 CNY ($0.274 USD)
			RawOutputCost:   2.0,
			InputCost:       1.0,
			OutputCost:      2.0,
			ModelRatio:      0.5,
			CompletionRatio: 2.0,
			BaseMultiplier:  1.0,
			Currency:        "USD",
			ExchangeRate:    1.0,
			FromUpstream:    true,
		},
		discountedModel: {
			ChannelID:       902,
			ChannelName:     "lumiya-0.4",
			Model:           discountedModel,
			QuotaType:       0,
			RawInputCost:    3.2, // 0.4x of ¥8 (> 0.2x ¥1.6)
			RawOutputCost:   11.2,
			InputCost:       3.2 / 7.3,
			OutputCost:      11.2 / 7.3,
			ModelRatio:      4.0,
			CompletionRatio: 3.5,
			BaseMultiplier:  0.4,
			Currency:        "CNY",
			ExchangeRate:    5.0,
			FromUpstream:    true,
		},
	}, false)

	updated, err := applyAuthoritativeModelPricesToSystem(model.ModelPriceSyncConfig{
		HighestPricePriority: true,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, updated, 4)

	cnyRatio, ok, _ := ratio_setting.GetModelRatio(cnyModel)
	require.True(t, ok)
	assert.InDelta(t, 1.0/7.3, cnyRatio, 1e-5, "CNY ¥2/1M must convert to USD ModelRatio ≈ 0.136986 instead of inflated 1.0")
	assert.InDelta(t, 4.0, ratio_setting.GetCompletionRatio(cnyModel), 1e-6)

	compareRatio, ok, _ := ratio_setting.GetModelRatio(compareModel)
	require.True(t, ok)
	assert.InDelta(t, 0.5, compareRatio, 1e-5, "USD $1/1M channel must win HighestPricePriority over CNY ¥2/1M ($0.274/1M)")

	discountedRatio, ok, _ := ratio_setting.GetModelRatio(discountedModel)
	require.True(t, ok)
	assert.InDelta(t, (3.2/7.3)/2.0, discountedRatio, 1e-5, "0.4x channel (¥3.2/1M) must win over 0.2x channel (¥1.6/1M) and sync its actual displayed USD ratio")

	unconfRatio, ok, _ := ratio_setting.GetModelRatio(unconfiguredModel)
	require.True(t, ok)
	assert.InDelta(t, 0.625, unconfRatio, 1e-5, "unconfigured 37.5 model must get sensible family fallback instead of 37.5 ($2,400/1M in TWD)")
}
