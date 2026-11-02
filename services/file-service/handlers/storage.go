package handlers

import (
	"math"
	"net/http"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/httputils"
	"github.com/gin-gonic/gin"
)

func GetStorageQuota() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		quota, err := db.GetStorageQuota(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch storage quota",
			})
			return
		}

		availableBytes := quota.QuotaBytes - quota.UsedBytes - quota.ReservedBytes

		var usagePercentage float64
		if quota.QuotaBytes > 0 {
			usagePercentage = math.Round(
				float64(quota.UsedBytes+quota.ReservedBytes)/
					float64(quota.QuotaBytes)*10000,
			) / 100
		}

		c.JSON(http.StatusOK, gin.H{
			"quotaBytes":      quota.QuotaBytes,
			"usedBytes":       quota.UsedBytes,
			"reservedBytes":   quota.ReservedBytes,
			"availableBytes":  availableBytes,
			"usagePercentage": usagePercentage,
		})
	}
}
