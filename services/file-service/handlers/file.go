package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/services/tasks"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	// "testing"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/httputils"
	SharedTasks "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/tasks"
	"github.com/gin-gonic/gin"
)

func ListFilesByParentId() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		publicParentID := c.Query("parent_id")

		var internalParentID *int64
		if publicParentID != "" {
			id, err := db.GetInternalID(c.Request.Context(), publicParentID, userId)
			if id == nil && err == nil {
				c.JSON(http.StatusNotFound, gin.H{"message": "Parent directory not found"})
				return
			} else if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to get internal ID", "error": err.Error()})
				return
			}

			internalParentID = id
		}

		files, err := db.GetFilesByParentId(c.Request.Context(), userId, internalParentID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve files from database", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, files)
	}
}

func Upload(uploader *utils.S3Uploader, taskService *tasks.TaskService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		entityType := c.Request.FormValue("entityType")
		publicParentID := c.Request.FormValue("parentId")

		var internalParentID *int64
		if publicParentID != "" {
			id, err := db.GetInternalID(c.Request.Context(), publicParentID, userId)
			if id == nil && err == nil {
				c.JSON(http.StatusNotFound, gin.H{"message": "Parent directory not found"})
				return
			} else if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to get internal ID", "error": err.Error()})
				return
			}

			internalParentID = id
		}

		publicId, err := utils.GenerateUniqueID(12)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate unique ID", "error": err.Error()})
			return
		}

		switch entityType {
		case "file":
			err = UploadFile(c, uploader, taskService, userId, publicId, internalParentID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to upload file", "error": err.Error()})
				return
			}
		case "folder":
			name := c.Request.FormValue("name")
			if name == "" {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Folder name is required"})
				return
			}
			err = UploadFolder(c, userId, publicId, name, internalParentID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to upload folder", "error": err.Error()})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid entityType, must be 'file' or 'folder'"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Upload Successful", "publicId": publicId})
	}
}

func UploadFile(c *gin.Context, uploader *utils.S3Uploader, taskService *tasks.TaskService, userId int64, publicId string, internalParentID *int64) error {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return err
	}
	defer file.Close()

	size := header.Size
	if size < 0 {
		return fmt.Errorf("invalid file size")
	}

	ctx := c.Request.Context()

	// 1. Create an individual storage reservation.
	reservationID, err := db.CreateStorageReservation(ctx, userId, size)
	if err != nil {
		return err
	}

	reservationPending := true

	// 2. Release this exact reservation if upload fails.
	defer func() {
		if reservationPending {
			// Best-effort cleanup. Log failures for monitoring/reconciliation.
			if err := db.ReleaseStorageReservationByID(context.Background(), reservationID); err != nil {
				log.Printf("Failed to release reservation %d for user %d: %v", reservationID, userId, err)
			}
		}
	}()

	baseName, ext := utils.ParseFilename(header.Filename)
	s3Key := utils.CreateS3Key(userId, publicId, ext)

	// 3. Upload file to S3.
	if err = uploader.UploadFile(file, header, s3Key); err != nil {
		return err
	}

	parentID := internalParentID

	entryData := models.EntryData{
		PublicId:    publicId,
		UserId:      userId,
		ParentId:    parentID,
		Name:        baseName,
		Type:        "FILE",
		ContentType: header.Header.Get("Content-Type"),
		Extension:   ext,
		Size:        size,
		S3Key:       sql.NullString{String: s3Key, Valid: true},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 4. Insert entry and finalize this exact reservation.
	newID, err := db.InsertEntryAndFinalizeStorage(c.Request.Context(), &entryData, reservationID)
	if err != nil {
		// S3 upload succeeded, but database persistence failed.
		// Try to remove the orphaned S3 object
		if cleanupErr := uploader.DeleteObject(s3Key, false); cleanupErr != nil {
			log.Printf("Failed to clean up S3 object %s: %v", s3Key, cleanupErr)
		}
		return err
	}

	// 5. Reservation is now FINALIZED.
	reservationPending = false

	// 6. Queue PDF embedding task.
	if entryData.ContentType == "application/pdf" {
		err = taskService.EnqueueGenerateEmbeddingTask(newID, s3Key)
		if err != nil {
			log.Printf("Failed to queue AI task: %v", err.Error())
		}
	}

	return nil
}

func UploadFolder(c *gin.Context, userId int64, publicId string, name string, internalParentID *int64) error {
	var parentID *int64
	if internalParentID != nil {
		parentID = internalParentID
	}

	entryData := models.EntryData{
		PublicId:    publicId,
		UserId:      userId,
		ParentId:    parentID,
		Name:        name,
		Type:        "FOLDER",
		ContentType: "application/x-directory",
		Extension:   "",
		Size:        0,
		S3Key:       sql.NullString{Valid: false},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := db.InsertEntryData(c.Request.Context(), &entryData)
	if err != nil {
		return err
	}

	return nil
}

type DeleteRequest struct {
	ParentPath string `json:"parentPath"`
	FileName   string `json:"fileName"`
	Type       string `json:"type"`
}

func DeleteContent(uploader *utils.S3Uploader) gin.HandlerFunc {
	return func(c *gin.Context) {
		publicID := c.Param("id")
		if publicID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid public ID"})
			return
		}

		userId, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		entityType, err := db.GetEntityType(c.Request.Context(), publicID, userId)
		if entityType == "" && err == nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "File or folder not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to get entity type", "error": err.Error()})
			return
		}

		switch entityType {
		case "file":
			err = db.DeleteFile(c.Request.Context(), publicID, userId)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to delete file", "error": err.Error()})
				return
			}
		case "folder":
			err = db.DeleteFolder(c.Request.Context(), publicID, userId)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to delete folder", "error": err.Error()})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid entity type"})
			return
		}

		if errors.Is(err, db.ErrEntryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "File or folder not found",
			})
			return
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Failed to delete entry",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Deleted successfully"})
	}
}

func DownloadFile(uploader *utils.S3Uploader) gin.HandlerFunc {
	return func(c *gin.Context) {
		publicID := c.Param("id")
		if publicID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid public ID"})
			return
		}

		userId, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		file, err := db.GetDownloadFile(c.Request.Context(), publicID, userId)
		if file == nil && err == nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "File not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to get file record", "error": err.Error()})
			return
		}

		if file.Type != "FILE" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot download a non-file entity"})
			return
		}

		filename := file.Name + "." + file.Extension
		url, err := uploader.GeneratePresignedURL(file.S3Key, 30*time.Second, filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate download URL", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"downloadURL": url})
	}
}

type SearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

func HandleSearch() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, err := httputils.GetUserIdHeader(c)
		if httputils.HandleUserIdHeaderError(c, err) {
			return
		}

		var req SearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request", "error": err.Error()})
			return
		}

		queryVector, err := SharedTasks.GenerateEmbedding(req.Query, config.AppConfig.OpenAiApiKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate embedding", "error": err.Error()})
			return
		}

		files, err := db.SearchByVector(c.Request.Context(), queryVector, req.Limit, userId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to search files", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, files)
	}
}
