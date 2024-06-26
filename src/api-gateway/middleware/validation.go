package middleware

import (
	"errors"
	"fmt"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/dto"
	"go.uber.org/zap"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
	"github.com/truemail-rb/truemail-go"
	"github.com/wwi21seb-projekt/errors-go/goerrors"
)

// contextKey is a type used for context keys to avoid conflicts with other packages' context keys.
type contextKey struct {
	name string
}

// Returns string representation of the context key.
func (c *contextKey) String() string {
	return c.name
}

var SanitizedPayloadKey = &contextKey{"sanitizedPayload"}
var badRequestError = &dto.ErrorDTO{Error: goerrors.BadRequest}

type Validator struct {
	SanitizeData func(data interface{}) error
	Validate     *validator.Validate
	VerifyEmail  func(email string) bool
}

// SanitizeData uses reflection to sanitize all string fields of a struct
func sanitizeData(policy *bluemonday.Policy, data interface{}) error {
	v := reflect.ValueOf(data)
	// Ensure that the provided data is a pointer
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("sanitizeData expects a pointer to a struct")
	}
	// Dereference the pointer to get the struct
	v = v.Elem()
	// Ensure that we now have a struct
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("sanitizeData expects a pointer to a struct, got a pointer to %v", v.Kind())
	}

	// Iterate over all fields of the struct
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		// Check if the field can be set
		if !field.CanSet() {
			continue
		}

		// Sanitize string fields
		if field.Kind() == reflect.String {
			originalText := field.String()
			sanitizedText := policy.Sanitize(strings.TrimSpace(originalText))
			field.SetString(sanitizedText)
		}

		// Recursively handle nested structs and pointers to structs
		if field.Kind() == reflect.Struct {
			_ = sanitizeData(policy, field.Addr().Interface())
		} else if field.Kind() == reflect.Ptr && field.Elem().Kind() == reflect.Struct {
			// Ensure the pointer is not nil before trying to sanitize
			if !field.IsNil() {
				_ = sanitizeData(policy, field.Interface())
			}
		}
	}
	return nil
}

// RegisterCustomValidators registers custom validators for our
// application-specific fields.
func registerCustomValidators(v *validator.Validate) {
	_ = v.RegisterValidation("username_validation", usernameValidation)
	_ = v.RegisterValidation("password_validation", passwordValidation)
	_ = v.RegisterValidation("post_validation", postValidation)
	//	_ = v.RegisterValidation("location_validation", locationValidation)
}

// usernameValidation defines the validation logic for a username.
// It ensures that the username matches a specific pattern.
func usernameValidation(fl validator.FieldLevel) bool {
	username := fl.Field().String()
	// Define the regular expression pattern for a valid username
	// The pattern allows a-z, A-Z, 0-9, ., -, and _
	pattern := `^[a-zA-Z0-9.\-_]+$`
	match, err := regexp.MatchString(pattern, username)
	if err != nil {
		return false
	}

	return match
}

// passwordValidation defines the validation logic for a password.
// It ensures that the password contains uppercase, lowercase, numeric, and special characters.
func passwordValidation(fl validator.FieldLevel) bool {
	var upperLetter, lowerLetter, number, specialChar bool

	value := fl.Field().String()
	for _, r := range value {
		if r > unicode.MaxASCII {
			return false
		}

		switch {
		case unicode.IsUpper(r):
			upperLetter = true
		case unicode.IsLower(r):
			lowerLetter = true
		case unicode.IsNumber(r):
			number = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			specialChar = true
		}
	}

	return upperLetter && lowerLetter && number && specialChar
}

// postValidation defines the validation logic for a post.
// It ensures that the post content is a valid UTF-8 encoded string.
func postValidation(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return utf8.ValidString(value)
}

func NewValidator() *Validator {
	configuration, _ := truemail.NewConfiguration(truemail.ConfigurationAttr{
		VerifierEmail:         "team@mail.server-alpha.tech",
		ValidationTypeDefault: "mx",
		SmtpFailFast:          true,
	})
	sanitizer := bluemonday.UGCPolicy()

	validate := validator.New()

	// Register custom validators
	registerCustomValidators(validate)

	return &Validator{
		SanitizeData: func(data interface{}) error { return sanitizeData(sanitizer, data) },
		Validate:     validate,
		VerifyEmail:  func(email string) bool { return truemail.IsValid(email, configuration) },
	}
}

func (m *Middleware) ValidateAndSanitizeStruct(objType interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		obj := reflect.New(reflect.TypeOf(objType)).Interface()

		if err := c.ShouldBindJSON(obj); err != nil {
			m.logger.Debugw("Failed to bind JSON", "error", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusBadRequest, badRequestError)
			return
		}

		if err := m.validator.SanitizeData(obj); err != nil {
			m.logger.Debugw("Failed to sanitize data", "error", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusBadRequest, badRequestError)
			return
		}

		if err := m.validator.Validate.Struct(obj); err != nil {
			var invalidValidationError *validator.InvalidValidationError
			if errors.As(err, &invalidValidationError) {
				m.logger.Error("Invalid validation error", "error", zap.Error(err))
				return
			}

			for _, err := range err.(validator.ValidationErrors) {
				m.logger.Debugw("Validation error",
					zap.String("Field", err.Field()),
					zap.String("Tag", err.Tag()),
					zap.String("ActualTag", err.ActualTag()),
					zap.String("Namespace", err.Namespace()),
					zap.String("Param", err.Param()),
					zap.Any("Value", err.Value()),
					zap.String("Type", err.Type().String()),
				)
			}

			c.AbortWithStatusJSON(http.StatusBadRequest, badRequestError)
			return
		}

		c.Set(SanitizedPayloadKey.String(), obj)
		c.Next()
	}
}
