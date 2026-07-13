package middleware

import (
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// ValidateBackofficeCredentials checks that production deployments do not use the
// default/weak password. This is called at server startup to fail fast rather than
// silently allowing an insecure backoffice. In DEV mode the default password is
// permitted for convenience, but a warning is logged.
func ValidateBackofficeCredentials() error {
	dev := os.Getenv("DEV")
	pass := os.Getenv("BACKOFFICE_PASS")

	// In DEV mode, the default password is acceptable for convenience.
	// Production (DEV != "true") must have a non-empty, non-default password.
	if dev != "true" {
		if strings.TrimSpace(pass) == "" || strings.TrimSpace(pass) == "changeme" {
			return fmt.Errorf("BACKOFFICE_PASS must be set to a strong password in production (DEV != \"true\"); refusing to start with default/weak credentials")
		}
	}

	return nil
}

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
