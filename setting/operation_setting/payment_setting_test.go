package operation_setting

import "testing"

func TestQuotePaymentAmountAppliesProviderCurrencyAndFees(t *testing.T) {
	got, err := QuotePaymentAmount(10, PaymentMethodSetting{
		Currency:     "TWD",
		ExchangeRate: 32,
		FeeRate:      0.03,
		FixedFee:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 334.6 {
		t.Fatalf("quoted amount = %v, want 334.6", got)
	}
}

func TestQuotePaymentAmountRejectsInvalidConfiguration(t *testing.T) {
	if _, err := QuotePaymentAmount(10, PaymentMethodSetting{ExchangeRate: 0}); err == nil {
		t.Fatal("expected invalid exchange rate error")
	}
}
