package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// 额度展示类型
const (
	QuotaDisplayTypeUSD    = "USD"
	QuotaDisplayTypeCNY    = "CNY"
	QuotaDisplayTypeNTD    = "NTD"
	QuotaDisplayTypeTokens = "TOKENS"
	QuotaDisplayTypeCustom = "CUSTOM"
)

type CurrencyRate struct {
	Symbol string  `json:"symbol"`
	Rate   float64 `json:"rate"`
}

type GeneralSetting struct {
	DocsLink            string `json:"docs_link"`
	PingIntervalEnabled bool   `json:"ping_interval_enabled"`
	PingIntervalSeconds int    `json:"ping_interval_seconds"`
	// 当前站点额度展示类型：USD / CNY / NTD / TOKENS / CUSTOM 或 currency_rates 中配置的币种代码
	QuotaDisplayType string `json:"quota_display_type"`
	// 自定义货币符号，用于 CUSTOM 展示类型
	CustomCurrencySymbol string `json:"custom_currency_symbol"`
	// 自定义货币与美元汇率（1 USD = X Custom）
	CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
	// 多币种汇率配置（1 USD = Rate <Currency>），key 为大写币种代码（如 USD、CNY、TWD、JPY、EUR）
	CurrencyRates map[string]CurrencyRate `json:"currency_rates"`
}

// 默认配置
var generalSetting = GeneralSetting{
	DocsLink:                   "https://docs.newapi.pro",
	PingIntervalEnabled:        false,
	PingIntervalSeconds:        60,
	QuotaDisplayType:           QuotaDisplayTypeUSD,
	CustomCurrencySymbol:       "¤",
	CustomCurrencyExchangeRate: 1.0,
	CurrencyRates:              map[string]CurrencyRate{},
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("general_setting", &generalSetting)
}

func GetGeneralSetting() *GeneralSetting {
	return &generalSetting
}

// GetEffectiveCurrencyRates returns the merged map of built-in defaults and
// admin-configured multi-currency rates. USD is always anchored at 1.
func GetEffectiveCurrencyRates() map[string]CurrencyRate {
	cnyRate := USDExchangeRate
	if cnyRate <= 0 {
		cnyRate = 7.3
	}
	ntdRate := NTDExchangeRate
	if ntdRate <= 0 {
		ntdRate = 32.0
	}
	rates := map[string]CurrencyRate{
		"USD": {Symbol: "$", Rate: 1},
		"CNY": {Symbol: "¥", Rate: cnyRate},
		"TWD": {Symbol: "NT$", Rate: ntdRate},
		"JPY": {Symbol: "¥", Rate: 150},
		"EUR": {Symbol: "€", Rate: 0.92},
	}
	for rawCode, entry := range generalSetting.CurrencyRates {
		code := strings.ToUpper(strings.TrimSpace(rawCode))
		if code == "" || code == "TOKENS" || code == "CUSTOM" {
			continue
		}
		if code == "NTD" {
			code = "TWD"
		}
		if code == "USD" {
			symbol := strings.TrimSpace(entry.Symbol)
			if symbol == "" {
				symbol = "$"
			}
			rates["USD"] = CurrencyRate{Symbol: symbol, Rate: 1}
			continue
		}
		if entry.Rate <= 0 {
			continue
		}
		symbol := strings.TrimSpace(entry.Symbol)
		if symbol == "" {
			if existing, ok := rates[code]; ok {
				symbol = existing.Symbol
			} else {
				symbol = code + " "
			}
		}
		rates[code] = CurrencyRate{Symbol: symbol, Rate: entry.Rate}
	}
	return rates
}

// IsCurrencyDisplay 是否以货币形式展示（非 TOKENS）
func IsCurrencyDisplay() bool {
	return generalSetting.QuotaDisplayType != QuotaDisplayTypeTokens
}

// IsCNYDisplay 是否以人民币展示
func IsCNYDisplay() bool {
	return generalSetting.QuotaDisplayType == QuotaDisplayTypeCNY
}

// GetQuotaDisplayType 返回额度展示类型
func GetQuotaDisplayType() string {
	return generalSetting.QuotaDisplayType
}

// GetCurrencySymbol 返回当前展示类型对应符号
func GetCurrencySymbol() string {
	displayType := strings.ToUpper(strings.TrimSpace(generalSetting.QuotaDisplayType))
	switch displayType {
	case QuotaDisplayTypeUSD:
		return "$"
	case QuotaDisplayTypeCNY:
		if rate, ok := generalSetting.CurrencyRates["CNY"]; ok && strings.TrimSpace(rate.Symbol) != "" {
			return strings.TrimSpace(rate.Symbol)
		}
		return "¥"
	case QuotaDisplayTypeNTD, "TWD":
		if rate, ok := generalSetting.CurrencyRates["TWD"]; ok && strings.TrimSpace(rate.Symbol) != "" {
			return strings.TrimSpace(rate.Symbol)
		}
		return "NT$"
	case QuotaDisplayTypeCustom:
		if generalSetting.CustomCurrencySymbol != "" {
			return generalSetting.CustomCurrencySymbol
		}
		return "¤"
	case QuotaDisplayTypeTokens, "":
		return ""
	default:
		if rate, ok := GetEffectiveCurrencyRates()[displayType]; ok && rate.Symbol != "" {
			return rate.Symbol
		}
		return displayType
	}
}

// GetUsdToCurrencyRate 返回 1 USD = X <currency> 的 X（TOKENS 不适用）
func GetUsdToCurrencyRate(usdToCny float64) float64 {
	displayType := strings.ToUpper(strings.TrimSpace(generalSetting.QuotaDisplayType))
	switch displayType {
	case QuotaDisplayTypeUSD:
		return 1
	case QuotaDisplayTypeCNY:
		if entry, ok := generalSetting.CurrencyRates["CNY"]; ok && entry.Rate > 0 {
			return entry.Rate
		}
		return usdToCny
	case QuotaDisplayTypeNTD, "TWD":
		if entry, ok := generalSetting.CurrencyRates["TWD"]; ok && entry.Rate > 0 {
			return entry.Rate
		}
		if entry, ok := generalSetting.CurrencyRates["NTD"]; ok && entry.Rate > 0 {
			return entry.Rate
		}
		if NTDExchangeRate > 0 {
			return NTDExchangeRate
		}
		return 1
	case QuotaDisplayTypeCustom:
		if generalSetting.CustomCurrencyExchangeRate > 0 {
			return generalSetting.CustomCurrencyExchangeRate
		}
		return 1
	case QuotaDisplayTypeTokens, "":
		return 1
	default:
		if entry, ok := GetEffectiveCurrencyRates()[displayType]; ok && entry.Rate > 0 {
			return entry.Rate
		}
		return 1
	}
}
