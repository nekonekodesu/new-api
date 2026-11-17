package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	MidjourneyImageAccessQueryParameter = "access"
	midjourneyImageAccessVersion        = "v1"
	midjourneyImageAccessLength         = 43
	maxMidjourneyImageIDLength          = 512
)

var ErrMidjourneyImageAccessInvalid = errors.New("midjourney image access is invalid")

func midjourneyImageAccessMessage(mjId string) []byte {
	return []byte(midjourneyImageAccessVersion + "\x00" + mjId)
}

// CreateMidjourneyImageAccess issues a stable capability bound to exactly one
// midjourney task ID. It contains no user or upstream data.
func CreateMidjourneyImageAccess(mjId string) (string, error) {
	mjId = strings.TrimSpace(mjId)
	if mjId == "" || len(mjId) > maxMidjourneyImageIDLength || common.CryptoSecret == "" {
		return "", ErrMidjourneyImageAccessInvalid
	}
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write(midjourneyImageAccessMessage(mjId))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// VerifyMidjourneyImageAccess verifies the capability binding without reading
// task, user, or token state. Signature comparison is constant-time.
func VerifyMidjourneyImageAccess(access, mjId string) bool {
	mjId = strings.TrimSpace(mjId)
	if mjId == "" || len(mjId) > maxMidjourneyImageIDLength ||
		len(access) != midjourneyImageAccessLength || common.CryptoSecret == "" {
		return false
	}
	actualSignature, err := base64.RawURLEncoding.Strict().DecodeString(access)
	if err != nil || len(actualSignature) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write(midjourneyImageAccessMessage(mjId))
	return hmac.Equal(actualSignature, mac.Sum(nil))
}

// BindMidjourneyImageAccess attaches the capability query parameter to a
// proxied image URL of the form <base>/mj/image/<mjId>.
func BindMidjourneyImageAccess(rawURL, mjId string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Opaque != "" {
		return "", ErrMidjourneyImageAccessInvalid
	}
	access, err := CreateMidjourneyImageAccess(mjId)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set(MidjourneyImageAccessQueryParameter, access)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
