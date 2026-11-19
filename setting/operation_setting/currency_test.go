package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUsdToCurrencyRateUsesSelectedCurrencyRate(t *testing.T) {
	originalType := generalSetting.QuotaDisplayType
	originalNTD := NTDExchangeRate
	originalRates := generalSetting.CurrencyRates
	t.Cleanup(func() {
		generalSetting.QuotaDisplayType = originalType
		NTDExchangeRate = originalNTD
		generalSetting.CurrencyRates = originalRates
	})

	generalSetting.CurrencyRates = map[string]CurrencyRate{}
	generalSetting.QuotaDisplayType = QuotaDisplayTypeNTD
	NTDExchangeRate = 32
	assert.Equal(t, 32.0, GetUsdToCurrencyRate(7.3))

	generalSetting.QuotaDisplayType = QuotaDisplayTypeCNY
	assert.Equal(t, 7.3, GetUsdToCurrencyRate(7.3))

	// Dynamic multi-currency table overrides and custom ISO currencies
	generalSetting.CurrencyRates = map[string]CurrencyRate{
		"USD": {Symbol: "$", Rate: 99}, // USD must remain anchored to 1
		"TWD": {Symbol: "NT$", Rate: 31.5},
		"JPY": {Symbol: "¥", Rate: 155},
		"HKD": {Symbol: "HK$", Rate: 7.8},
	}

	effective := GetEffectiveCurrencyRates()
	assert.Equal(t, 1.0, effective["USD"].Rate)
	assert.Equal(t, 31.5, effective["TWD"].Rate)
	assert.Equal(t, 155.0, effective["JPY"].Rate)
	assert.Equal(t, "HK$", effective["HKD"].Symbol)
	assert.Equal(t, 7.8, effective["HKD"].Rate)

	generalSetting.QuotaDisplayType = "HKD"
	assert.Equal(t, 7.8, GetUsdToCurrencyRate(7.3))
	assert.Equal(t, "HK$", GetCurrencySymbol())

	generalSetting.QuotaDisplayType = QuotaDisplayTypeNTD
	assert.Equal(t, 31.5, GetUsdToCurrencyRate(7.3))
}
