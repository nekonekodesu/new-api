package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECPayCheckMacValueMatchesOfficialVectorAndIgnoresExistingSignature(t *testing.T) {
	// Official ECPay public test vector from https://developers.ecpay.com.tw/2902/
	officialFields := map[string]string{
		"ChoosePayment":     "ALL",
		"EncryptType":       "1",
		"ItemName":          "Apple iphone 15",
		"MerchantID":        "3002607",
		"MerchantTradeDate": "2023/03/12 15:30:23",
		"MerchantTradeNo":   "ecpay20230312153023",
		"PaymentType":       "aio",
		"ReturnURL":         "https://www.ecpay.com.tw/receive.php",
		"TotalAmount":       "30000",
		"TradeDesc":         "促銷方案",
		"CheckMacValue":     "stale-signature",
	}
	got := ECPayCheckMacValue(officialFields, "pwFHCqoQZGmho4w6", "EkRm7iFT261dpevs")
	assert.Equal(t, "6C51C9E6888DE861FD62FB1DD17029FC742634498FD813DC43D4243B5685B840", got)

	officialFields["CheckMacValue"] = got
	assert.Equal(t, got, ECPayCheckMacValue(officialFields, "pwFHCqoQZGmho4w6", "EkRm7iFT261dpevs"))
}

func TestNewebPayEncryptDecryptCheckValueAndMPG20JSONNotify(t *testing.T) {
	prev := paymentSetting
	t.Cleanup(func() { paymentSetting = prev })

	key := "12345678901234567890123456789012"
	iv := "1234567890123456"
	paymentSetting.NewebPayHashKey = key
	paymentSetting.NewebPayHashIV = iv

	encoded, err := NewebPayEncryptInfo("Status=SUCCESS&MerchantID=MS123&MerchantOrderNo=ORDER1&Amt=100", key, iv)
	require.NoError(t, err)
	check := NewebPayCheckValue(encoded, key, iv)
	require.True(t, NewebPayVerifyCheckValue(encoded, check, key, iv))

	strValues, err := NewebPayDecodeTradeInfo(encoded, check)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", strValues.Get("Status"))
	assert.Equal(t, "ORDER1", strValues.Get("MerchantOrderNo"))
	assert.Equal(t, "100", strValues.Get("Amt"))

	// Official NewebPay MPG 2.0 RespondType=JSON NotifyURL decrypted structure
	officialJSON := `{"Status":"SUCCESS","Message":"授權成功","Result":{"MerchantID":"MS345678","Amt":320,"TradeNo":"26112603450012345","MerchantOrderNo":"NP0123456789ABCDEFGH","PaymentType":"CREDIT","RespondType":"JSON"}}`
	jsonCipher, err := NewebPayEncryptInfo(officialJSON, key, iv)
	require.NoError(t, err)
	jsonSha := NewebPayCheckValue(jsonCipher, key, iv)

	jsonValues, err := NewebPayDecodeTradeInfo(jsonCipher, jsonSha)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", jsonValues.Get("Status"))
	assert.Equal(t, "MS345678", jsonValues.Get("MerchantID"))
	assert.Equal(t, "NP0123456789ABCDEFGH", jsonValues.Get("MerchantOrderNo"))
	assert.Equal(t, "320", jsonValues.Get("Amt"))
	assert.Equal(t, "26112603450012345", jsonValues.Get("TradeNo"))
}
