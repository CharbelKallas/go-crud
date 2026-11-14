// Package handlers contains the HTTP handlers of the API.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	dec := json.NewDecoder(c.Request.Body)
	if err := dec.Decode(dst); err != nil {
		respondError(c, http.StatusBadRequest, describeBindError(err))
		return false
	}
	// The body must be exactly one JSON value (trailing whitespace is fine).
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		respondError(c, http.StatusBadRequest, "invalid JSON body: unexpected data after the JSON object")
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
		// Names the field as the client spelled it, e.g. "price".
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return typeErr.Field + " must be " + jsonKind(typeErr.Type.Kind())
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

// jsonKind describes the JSON value expected for a Go kind.
func jsonKind(k reflect.Kind) string {
	switch k {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "a number"
	default:
		return "a different type"
	}
}
