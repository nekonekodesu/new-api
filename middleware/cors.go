package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	// Token-authed public endpoints only; no cookies cross-origin (SameSite=Strict
	// keeps sessions same-site, and credentials on ACAO:* are browser-rejected).
	config.AllowAllOrigins = true
	config.AllowCredentials = false
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{
		"Accept",
		"Accept-Language",
		"Authorization",
		"Content-Type",
		"X-Requested-With",
		"X-Auth-Session",
		"anthropic-version",
		"mj-api-key",
		"mj-api-secret",
		"x-api-key",
		"x-goog-api-key",
	}
	return cors.New(config)
}

func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
