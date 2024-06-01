package helper

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func ExtractPaginationFromContext(c *gin.Context) (int, int) {
	offset, limit := 0, 10

	// Check if offset and limit are provided and in the correct format
	// If not, return the default values
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = o
	}
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	return offset, limit
}
