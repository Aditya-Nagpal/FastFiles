package routes

import (
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/handlers"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/services/tasks"
	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine, s3Uploader *utils.S3Uploader, taskService *tasks.TaskService) {
	r.GET("/list", handlers.ListFilesByParentId())
	r.POST("/upload", handlers.Upload(s3Uploader, taskService))
	r.DELETE("/delete/:id", handlers.DeleteContent(s3Uploader))
	r.GET("/download/:id", handlers.DownloadFile(s3Uploader))
	r.POST("/search", handlers.HandleSearch())
}
