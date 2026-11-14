// Package handlers contains the HTTP handlers of the API.
package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// errorResponse is the body of every error response.
type errorResponse struct {
	Message string `json:"message"`
}

func respondError(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, errorResponse{Message: msg})
}

func logErr(c *gin.Context, err error) {
	slog.ErrorContext(c.Request.Context(), "request failed",
		"method", c.Request.Method, "path", c.FullPath(), "error", err)
}

// respondInternal logs the real error and hides it from the client.
func respondInternal(c *gin.Context, err error) {
	logErr(c, err)
	respondError(c, http.StatusInternalServerError, "internal server error")
}

// bindJSON decodes and validates the body, answering 400 on failure.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		respondError(c, http.StatusBadRequest, describeBindError(err))
		return false
	}
	return true
}

func describeBindError(err error) string {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return "request body too large"
		}
		return "invalid JSON body"
	}
	msgs := make([]string, 0, len(verrs))
	for _, fe := range verrs {
		switch fe.Tag() {
		case "required":
			msgs = append(msgs, fe.Field()+" is required")
		case "email":
			msgs = append(msgs, fe.Field()+" must be a valid email address")
		case "min", "gte":
			msgs = append(msgs, fmt.Sprintf("%s must be at least %s", fe.Field(), fe.Param()))
		case "max", "lte":
			msgs = append(msgs, fmt.Sprintf("%s must be at most %s", fe.Field(), fe.Param()))
		default:
			msgs = append(msgs, fe.Field()+" is invalid")
		}
	}
	return strings.Join(msgs, "; ")
}
