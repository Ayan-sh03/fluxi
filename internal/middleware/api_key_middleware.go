package middleware

import (
	"fmt"
	"net/http"
	"scrapper/internal/db"

	"github.com/gin-gonic/gin"
)

func APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		//get the api key from the request
		fmt.Println("APIKeyMiddleware")
		apiKey := c.GetHeader("X-API-KEY")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - No API key provided"})
			c.Abort()
			return
		}

		// Get user_id with proper type assertion and existence check
		userId, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found"})
			c.Abort()
			return
		}

		//check if the api key is valid
		if !IsAPIKeyValid(userId.(string), apiKey) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - Invalid API key"})
			c.Abort()
			return
		}

		c.Next()

	}
}
func IsAPIKeyValid(userId, apiKey string) bool {
	//check if the api key is valid and exists since one user can have multiple api keys
	ok, err := db.Db.CheckAPIKeyExists(apiKey, userId)

	if err != nil {

		fmt.Println("err", err)
		return false
	}
	fmt.Println("ok", ok)

	if !ok {
		return false
	}

	return true
}
