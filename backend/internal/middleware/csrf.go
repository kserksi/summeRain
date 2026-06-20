package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/response"
	"github.com/summerain/image-gallery/internal/pkg/token"
	"github.com/summerain/image-gallery/internal/repository"
)

type CSRFMiddleware struct {
	sessionRepo *repository.SessionRepo
}

func NewCSRFMiddleware(sessionRepo *repository.SessionRepo) *CSRFMiddleware {
	return &CSRFMiddleware{sessionRepo: sessionRepo}
}

func (m *CSRFMiddleware) Validate() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			c.Next()
			return
		}

		sessionID := GetSessionID(c)
		if sessionID == 0 {
			c.Next()
			return
		}

		csrfHeader := c.GetHeader("X-CSRF-Token")
		if csrfHeader == "" {
			response.Error(c, errcode.New(4035, "CSRF token required", 403))
			return
		}

		csrfHash := token.SHA256(csrfHeader)
		csrf, err := m.sessionRepo.FindCSRFBySessionAndHash(sessionID, csrfHash)
		if err != nil {
			response.Error(c, errcode.New(4036, "Invalid CSRF token", 403))
			return
		}

		m.sessionRepo.RenewCSRFExpiry(csrf.ID)
		c.Next()
	}
}
