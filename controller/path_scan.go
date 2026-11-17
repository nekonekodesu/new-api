package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func GetNetworkSecurityBans(c *gin.Context) {
	bans := middleware.GetActivePathScanBans()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": bans})
}

func DeleteNetworkSecurityBans(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	removed := middleware.DeletePathScanBan(ip)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": removed})
}
