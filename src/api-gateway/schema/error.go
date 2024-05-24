package schema

import "github.com/wwi21seb-projekt/errors-go/goerrors"

// ErrorDTO is a struct that represents an error response
// Error is the custom error, see CustomError
type ErrorDTO struct {
	Error *goerrors.CustomError `json:"error"`
}
