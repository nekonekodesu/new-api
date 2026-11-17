package operation_setting

import "testing"

func TestGetUsdToCurrencyRateUsesSelectedCurrencyRate(t *testing.T) {
	originalType := generalSetting.QuotaDisplayType
	originalNTD := NTDExchangeRate
	t.Cleanup(func() {
		generalSetting.QuotaDisplayType = originalType
		NTDExchangeRate = originalNTD
	})

	generalSetting.QuotaDisplayType = QuotaDisplayTypeNTD
	NTDExchangeRate = 32
	if got := GetUsdToCurrencyRate(7.3); got != 32 {
		t.Fatalf("NTD rate = %v, want 32", got)
	}

	generalSetting.QuotaDisplayType = QuotaDisplayTypeCNY
	if got := GetUsdToCurrencyRate(7.3); got != 7.3 {
		t.Fatalf("CNY rate = %v, want 7.3", got)
	}
}
