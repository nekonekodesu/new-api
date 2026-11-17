package operation_setting

import "testing"

func TestECPayCheckMacValueIsStableAndIgnoresExistingSignature(t *testing.T) {
	fields := map[string]string{
		"MerchantID":      "2000132",
		"MerchantTradeNo": "ORDER123",
		"CheckMacValue":   "old-value",
	}
	got := ECPayCheckMacValue(fields, "1234567890123456", "1234567890123456")
	if got == "" || got == "old-value" {
		t.Fatalf("unexpected ECPay signature %q", got)
	}
	fields["CheckMacValue"] = got
	if second := ECPayCheckMacValue(fields, "1234567890123456", "1234567890123456"); second != got {
		t.Fatalf("signature changed after adding existing signature: %q != %q", second, got)
	}
}
