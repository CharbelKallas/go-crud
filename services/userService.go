package services

import (
	"github.com/gin-gonic/gin"
	"go-crud/auth"
	"go-crud/models"
	"go-crud/repositories"
	"log"
	"net/http"
)

func Signup(c *gin.Context) {
	var user models.User
	err := c.ShouldBindJSON(&user)

	if err != nil {
		log.Println(err)
		c.IndentedJSON(400, gin.H{"Error": "Invalid Inputs "})
		c.Abort()
		return
	}

	err = user.HashPassword(user.Password)
	if err != nil {
		log.Println(err.Error())
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"Error": "Error Hashing Password"})
		c.Abort()
		return
	}

	err = repositories.CreateUserRecord(user)
	if err != nil {
		log.Println(err)
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"Error": "Error Creating User"})
		c.Abort()
		return
	}

	c.IndentedJSON(http.StatusOK, gin.H{"Message": "Successfully Register"})
}

type LoginResponse struct {
	Token        string
	RefreshToken string
}

type LoginPayload struct {
	Email    string
	Password string
}

func Login(c *gin.Context) {
	var payload LoginPayload

	err := c.ShouldBindJSON(&payload)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "Invalid Inputs"})
		c.Abort()
		return
	}

	user, err := repositories.UserByEmail(payload.Email)

	if err != nil {
		log.Println(err)
		c.IndentedJSON(http.StatusUnauthorized, gin.H{"message": "Invalid User Credentials"})
		c.Abort()
		return
	}

	err = user.CheckPassword(payload.Password)
	if err != nil {
		log.Println(err)
		c.IndentedJSON(http.StatusUnauthorized, gin.H{"message": "Invalid User Credentials"})
		c.Abort()
		return
	}

	jwtWrapper := auth.JwtWrapper{
		SecretKey:         "undersecretary",
		Issuer:            "AuthService",
		ExpirationMinutes: 1,
		ExpirationHours:   12,
	}

	signedToken, err := jwtWrapper.GenerateToken(user.Email)
	if err != nil {
		log.Println(err)
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "Error Signing Token"})
		c.Abort()
		return
	}

	signedRefreshToken, err := jwtWrapper.RefreshToken(user.Email)
	if err != nil {
		log.Println(err)
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "Error Signing Token"})
		c.Abort()
		return
	}

	tokenResponse := LoginResponse{
		Token:        signedToken,
		RefreshToken: signedRefreshToken,
	}

	c.IndentedJSON(200, tokenResponse)
}
