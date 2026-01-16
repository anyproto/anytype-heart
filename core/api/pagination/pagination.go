package pagination

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Config holds pagination configuration options.
type Config struct {
	DefaultPage     int
	DefaultPageSize int
	MinPageSize     int
	MaxPageSize     int
}

// New creates Gin middleware for pagination with the provided Config.
func New(cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := getIntQueryParam(c, QueryParamOffset, cfg.DefaultPage)
		size := getIntQueryParam(c, QueryParamLimit, cfg.DefaultPageSize)

		if size < cfg.MinPageSize || size > cfg.MaxPageSize {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("limit must be between %d and %d", cfg.MinPageSize, cfg.MaxPageSize),
			})
			return
		}

		c.Set(QueryParamOffset, page)
		c.Set(QueryParamLimit, size)

		c.Next()
	}
}

// getIntQueryParam retrieves an integer query parameter or falls back to a default value.
func getIntQueryParam(c *gin.Context, key string, defaultValue int) int {
	valStr := c.DefaultQuery(key, strconv.Itoa(defaultValue))
	val, err := strconv.Atoi(valStr)
	if err != nil || val < 0 {
		return defaultValue
	}
	return val
}

// RespondWithPagination sends a paginated JSON response.
func RespondWithPagination[T any](c *gin.Context, statusCode int, data []T, total int, offset int, limit int, hasMore bool) {
	c.JSON(statusCode, PaginatedResponse[T]{
		Data: data,
		Pagination: PaginationMeta{
			Total:   total,
			Offset:  offset,
			Limit:   limit,
			HasMore: hasMore,
		},
	})
}

// Paginate slices the records based on the offset and limit, and determines if more records are available.
func Paginate[T any](records []T, offset int, limit int) ([]T, bool) {
	if offset < 0 || limit < 1 {
		return []T{}, len(records) > 0
	}

	total := len(records)
	if offset > total {
		offset = total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	paginated := records[offset:end]
	hasMore := end < total

	return paginated, hasMore
}
