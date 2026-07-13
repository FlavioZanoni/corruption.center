package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CSRFProtection requires the HX-Request header on protected routes. This defends
// against cross-site form POST attacks: a malicious page cannot auto-submit a
// <form> across origins because the browser blocks custom headers on XHR in
// cross-origin contexts. Htmx, however, is same-origin JavaScript and can set
// custom headers freely, so it always includes HX-Request: true.
//
// This is a defense-in-depth layer for basic-auth backoffice POSTs that lack
// a CSRF token field. The backoffice uses htmx for all interactive forms, so
// rejecting requests that lack HX-Request has no legitimate cost: a real
// backoffice operator always submits via htmx, never via a bare HTML form.
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only protect state-changing requests (POST, PUT, PATCH, DELETE).
		// GET is safe and should never be blocked.
		if c.Request.Method != http.MethodPost &&
			c.Request.Method != http.MethodPut &&
			c.Request.Method != http.MethodPatch &&
			c.Request.Method != http.MethodDelete {
			c.Next()
			return
		}

		// Require the HX-Request header. This header can only be set by same-origin
		// JavaScript (htmx); cross-site HTML forms cannot set custom headers.
		if c.Request.Header.Get("HX-Request") != "true" {
			c.JSON(http.StatusForbidden, gin.H{"error": "CSRF protection: cross-origin form submission rejected"})
			c.Abort()
			return
		}

		c.Next()
	}
}
