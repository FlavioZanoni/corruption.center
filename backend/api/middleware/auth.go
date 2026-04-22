package middleware

import (
	"os"

	"github.com/gin-gonic/gin"
)

func BackofficeBasicAuth() gin.HandlerFunc {
	user := os.Getenv("BACKOFFICE_USER")
	pass := os.Getenv("BACKOFFICE_PASS")
	if user == "" {
		user = "admin"
	}
	if pass == "" {
		pass = "changeme"
	}
	return gin.BasicAuth(gin.Accounts{user: pass})
}
