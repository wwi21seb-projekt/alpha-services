package schema

import (
	"encoding/json"

	"github.com/wwi21seb-projekt/errors-go/goerrors"
)

// ErrorDTO is a struct that represents an error response
// Error is the custom error, see CustomError
type ErrorDTO struct {
	Error *goerrors.CustomError `json:"error"`
}

// MarshalJSON is a custom JSON marshaller for the ErrorDTO struct
func (e *ErrorDTO) MarshalJSON() ([]byte, error) {
	type Alias ErrorDTO
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	})
}
