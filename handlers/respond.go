// Package handlers contains the HTTP handlers of the API.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
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

// normalizer is implemented by payloads that clean their fields (for
// example trimming whitespace) before validation.
type normalizer interface {
	normalize()
}

// bindJSON decodes the body, normalizes it, then validates it, answering
// 400 on failure. Normalizing first means rules such as "required" and
// "email" apply to the value that is actually stored.
func bindJSON(c *gin.Context, dst any) bool {
	if c.Request.Body == nil {
		respondError(c, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if err := json.NewDecoder(c.Request.Body).Decode(dst); err != nil {
		respondError(c, http.StatusBadRequest, describeBindError(err))
		return false
	}
	if n, ok := dst.(normalizer); ok {
		n.normalize()
	}
	if err := binding.Validator.ValidateStruct(dst); err != nil {
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
			msgs = append(msgs, fmt.Sprintf("%s must be at least %s%s", fe.Field(), fe.Param(), lengthUnit(fe)))
		case "max", "lte":
			msgs = append(msgs, fmt.Sprintf("%s must be at most %s%s", fe.Field(), fe.Param(), lengthUnit(fe)))
		default:
			msgs = append(msgs, fe.Field()+" is invalid")
		}
	}
	return strings.Join(msgs, "; ")
}

// lengthUnit names the unit of a min/max rule: on strings the validator
// counts characters, on numbers it compares the value.
func lengthUnit(fe validator.FieldError) string {
	if fe.Kind() == reflect.String {
		return " characters"
	}
	return ""
}
