package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/auth-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/auth-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/auth-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/auth-service/services/sqs"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/hash"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/jwt"
	"github.com/gin-gonic/gin"
)

func Register(c *gin.Context) {
	var req models.User
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request", "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !req.TermsAndPrivacy {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Please accept terms and privacy policy"})
		return
	}

	password := strings.TrimSpace(req.Password)

	// Hash password
	hashedPassword, err := hash.HashPassword(password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Could not hash password", "error": err.Error()})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Password = string(hashedPassword)
	req.Name = strings.TrimSpace(req.Name)
	req.AlternateEmail = strings.ToLower(strings.TrimSpace(req.AlternateEmail))
	req.ContactNumber = strings.TrimSpace(req.ContactNumber)
	req.About = strings.TrimSpace(req.About)

	if req.Email == req.AlternateEmail {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Alternate email cannot be same as email"})
		return
	}

	// Insert user in db
	if err := db.RegisterUser(ctx, req); err != nil {
		// Check user already exists
		fmt.Println(err.Error())
		if err.Error() == "EMAIL_ALREADY_EXISTS" {
			c.JSON(http.StatusConflict, gin.H{"message": "Email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Could not create user", "error": err.Error()})
		return
	}

	if err := sqs.PublishSignupSuccessEmail(ctx, req.Email, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Could not send signup email", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": req, "message": "User registered successfully"})
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request", "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := strings.TrimSpace(req.Password)

	userLogin, err := db.GetUserLoginDetails(ctx, email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Error in getting login details", "error": err.Error()})
		return
	} else if userLogin == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Invalid email or password"})
		return
	}

	var hashedPassword = userLogin.Password
	if hashedPassword == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid email or password"})
		return
	}

	if !hash.CheckPasswordHash(password, hashedPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid email or password"})
		return
	}

	// Generate access token short-lived
	accessToken, err := jwt.GenerateWithExpiry(userLogin.Id, config.AppConfig.JwtSecret, 2*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Could not access generate token", "error": err.Error()})
		return
	}

	// Generate refresh token long-lived
	refreshToken, err := jwt.GenerateWithExpiry(userLogin.Id, config.AppConfig.JwtSecret, 7*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Could not refresh generate token", "error": err.Error()})
		return
	}

	// Set refresh token in secure HttpOnly cookie
	c.SetCookie("refreshToken", refreshToken, 7*24*60*60, "/", "localhost", false, true)

	c.JSON(http.StatusOK, gin.H{
		"userId":       userLogin.Id,
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	})
}

func RefreshToken(c *gin.Context) {
	refreshToken, err := c.Cookie("refreshToken")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "No refresh token found"})
		return
	}

	claims, err := jwt.Verify(refreshToken, config.AppConfig.JwtSecret)
	if err != nil || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid or expired refresh token"})
		return
	}

	newAccessToken, err := jwt.GenerateWithExpiry(claims.UserId, config.AppConfig.JwtSecret, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Could not generate new access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"accessToken": newAccessToken})
}

func Logout(c *gin.Context) {
	c.SetCookie("refreshToken", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
