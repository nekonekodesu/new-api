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

func TestNewebPayEncryptDecryptAndCheckValue(t *testing.T) {
	key := "12345678901234567890123456789012"
	iv := "1234567890123456"
	encoded, err := NewebPayEncryptInfo("MerchantOrderNo=ORDER1&Amt=100", key, iv)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := NewebPayDecryptInfo(encoded, key, iv)
	if err != nil || plain != "MerchantOrderNo=ORDER1&Amt=100" {
		t.Fatalf("decrypt = %q, err=%v", plain, err)
	}
	check := NewebPayCheckValue(encoded, key, iv)
	if !NewebPayVerifyCheckValue(encoded, check, key, iv) {
		t.Fatal("expected CheckValue verification to pass")
	}
}
