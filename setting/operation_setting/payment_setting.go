package operation_setting

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	// Per-channel quote settings. Rates are provider-currency units per USD.
	PaymentMethods  map[string]PaymentMethodSetting `json:"payment_methods"`
	ECPayMerchantID string                          `json:"ecpay_merchant_id"`
	ECPayHashKey    string                          `json:"ecpay_hash_key"`
	ECPayHashIV     string                          `json:"ecpay_hash_iv"`
	ECPayEndpoint   string                          `json:"ecpay_endpoint"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`

	// 易支付签名方式：MD5 使用 EpayKey；RSA 使用商户私钥签名请求、平台公钥验证回调。
	EpaySignType           string `json:"epay_sign_type"`
	EpayMerchantPrivateKey string `json:"epay_merchant_private_key"`
	EpayPlatformPublicKey  string `json:"epay_platform_public_key"`
}

// ECPayCheckMacValue returns the SHA256 signature used by ECPay's form API.
// Secrets are passed by the caller and are never logged or persisted by this helper.
func ECPayCheckMacValue(fields map[string]string, hashKey, hashIV string) string {
	values := make([]string, 0, len(fields))
	for key, value := range fields {
		if strings.EqualFold(key, "CheckMacValue") {
			continue
		}
		values = append(values, url.QueryEscape(key)+"="+url.QueryEscape(value))
	}
	sort.Strings(values)
	payload := "HashKey=" + url.QueryEscape(hashKey) + "&" + strings.Join(values, "&") + "&HashIV=" + url.QueryEscape(hashIV)
	payload = strings.ToLower(payload)
	payload = strings.ReplaceAll(payload, "%2d", "-")
	payload = strings.ReplaceAll(payload, "%5f", "_")
	hash := sha256.Sum256([]byte(payload))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

type PaymentMethodSetting struct {
	Currency     string  `json:"currency"`
	ExchangeRate float64 `json:"exchange_rate"`
	FeeRate      float64 `json:"fee_rate"`
	FixedFee     float64 `json:"fixed_fee"`
}

const CurrentComplianceTermsVersion = "v1"

const (
	EpaySignTypeMD5 = "MD5"
	EpaySignTypeRSA = "RSA"

	EpaySignTypeOptionKey           = "payment_setting.epay_sign_type"
	EpayMerchantPrivateKeyOptionKey = "payment_setting.epay_merchant_private_key"
	EpayPlatformPublicKeyOptionKey  = "payment_setting.epay_platform_public_key"
)

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
	PaymentMethods: map[string]PaymentMethodSetting{},
	// 没有保存过签名方式的站点按 MD5 运行，升级后行为不变。
	EpaySignType: EpaySignTypeMD5,
}

// QuotePaymentAmount converts a USD amount into the provider's currency and
// applies the configured percentage and fixed fees. Invalid rates are rejected
// instead of silently charging a different amount.
func QuotePaymentAmount(amountUSD float64, method PaymentMethodSetting) (float64, error) {
	if amountUSD <= 0 || method.ExchangeRate <= 0 || method.FeeRate < 0 || method.FixedFee < 0 {
		return 0, common.NewMessage("Invalid payment currency or fee configuration")
	}
	amount := amountUSD * method.ExchangeRate
	amount += amount*method.FeeRate + method.FixedFee
	return amount, nil
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}

// ValidateEpaySignOption checks a change to the Epay signing settings against
// the saved ones. While RSA is active, both keys must parse; under MD5 only a
// newly entered key is checked.
func ValidateEpaySignOption(key, value string) error {
	if key != EpaySignTypeOptionKey && key != EpayMerchantPrivateKeyOptionKey && key != EpayPlatformPublicKeyOptionKey {
		return nil
	}
	// Saved values are written under OptionMapRWMutex by the option update path.
	common.OptionMapRWMutex.RLock()
	signType := paymentSetting.EpaySignType
	privateKey := paymentSetting.EpayMerchantPrivateKey
	publicKey := paymentSetting.EpayPlatformPublicKey
	common.OptionMapRWMutex.RUnlock()
	switch key {
	case EpaySignTypeOptionKey:
		if value != EpaySignTypeMD5 && value != EpaySignTypeRSA {
			return common.NewMessage("Epay signature type must be MD5 or RSA")
		}
		signType = value
	case EpayMerchantPrivateKeyOptionKey:
		privateKey = value
	case EpayPlatformPublicKeyOptionKey:
		publicKey = value
	}
	rsaActive := signType == EpaySignTypeRSA
	if rsaActive || (key == EpayMerchantPrivateKeyOptionKey && strings.TrimSpace(value) != "") {
		if _, err := epay.ParseRSAPrivateKey(privateKey); err != nil {
			return common.NewMessage("The Epay merchant private key is missing or invalid")
		}
	}
	if rsaActive || (key == EpayPlatformPublicKeyOptionKey && strings.TrimSpace(value) != "") {
		if _, err := epay.ParseRSAPublicKey(publicKey); err != nil {
			return common.NewMessage("The Epay platform public key is missing or invalid")
		}
	}
	return nil
}
