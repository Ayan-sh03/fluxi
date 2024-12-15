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

		//check if the api key is valid
		if !IsAPIKeyValid(apiKey) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - Invalid API key"})
			c.Abort()
			return
		}

		c.Next()

	}
}
func IsAPIKeyValid(apiKey string) bool {
	//check if the api key is valid and exists since one user can have multiple api keys
	ok, err := db.Db.CheckAPIKeyExists(apiKey)

	if err != nil {

		fmt.Println("err", err)
		return false
	}

	if !ok {
		return false
	}

	return true
}
