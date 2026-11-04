package handlers

import (
	"net/http"
	"strings"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/httputils"
	SharedTasks "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/tasks"
	"github.com/gin-gonic/gin"
)

type SmartSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

func HandleSmartSearch() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		var req SmartSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request", "error": err.Error()})
			return
		}

		req.Query = strings.TrimSpace(req.Query)

		if req.Query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Search query cannot be empty"})
			return
		}

		if req.Limit <= 0 {
			req.Limit = 10
		}

		if req.Limit > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Limit cannot exceed 100"})
			return
		}

		queryVector, err := SharedTasks.GenerateEmbedding(req.Query, config.AppConfig.OpenAiApiKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate embedding", "error": err.Error()})
			return
		}

		files, err := db.SearchByVector(c.Request.Context(), userID, queryVector, req.Limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to search files", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total":   len(files),
			"results": files,
		})
	}
}

type NamedSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

func HandleNamedSearch() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		var req NamedSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request", "error": err.Error()})
			return
		}

		req.Query = strings.TrimSpace(req.Query)

		if req.Query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Search query cannot be empty"})
			return
		}

		if req.Limit <= 0 {
			req.Limit = 20
		}

		if req.Limit > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Limit cannot exceed 100"})
			return
		}

		results, err := db.SearchByName(c.Request.Context(), userID, req.Query, req.Limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to search files", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total":   len(results),
			"results": results,
		})
	}
}
