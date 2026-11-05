package handlers

import (
	"net/http"
	"strconv"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/httputils"
	"github.com/gin-gonic/gin"
)

type UpdateStarredRequest struct {
	IsStarred *bool `json:"is_starred" binding:"required"`
}

func HandleUpdateStarred() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		publicID := c.Param("id")
		if publicID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Entry ID is required"})
			return
		}

		var req UpdateStarredRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body", "error": err.Error()})
			return
		}

		err = db.UpdateStarredStatus(c.Request.Context(), userID, publicID, *req.IsStarred)

		if err != nil {
			if err == db.ErrEntryNotFound {
				c.JSON(http.StatusNotFound, gin.H{"message": "Entry not found"})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update starred status", "error": err.Error()})
			return
		}

		message := "Entry unstarred successfully"
		if *req.IsStarred {
			message = "Entry starred successfully"
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    message,
			"is_starred": *req.IsStarred,
		})
	}
}

func HandleGetStarredEntries() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		limit := 20
		offset := 0

		if limitParam := c.Query("limit"); limitParam != "" {
			parsedLimit, err := strconv.Atoi(limitParam)
			if err != nil || parsedLimit < 1 || parsedLimit > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Limit must be between 1 and 100"})
				return
			}
			limit = parsedLimit
		}

		if offsetParam := c.Query("offset"); offsetParam != "" {
			parsedOffset, err := strconv.Atoi(offsetParam)
			if err != nil || parsedOffset < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Offset must be a non-negative integer"})
				return
			}
			offset = parsedOffset
		}

		results, err := db.GetStarredEntries(c.Request.Context(), userID, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to fetch starred entries", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total":   len(results),
			"results": results,
		})
	}
}
