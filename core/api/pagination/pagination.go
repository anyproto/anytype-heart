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
	// OnInvalidLimit answers an out-of-range `limit`. It exists because the
	// two API versions refuse in different envelopes and this middleware is
	// shared: v1 keeps the bare {"error": …} below, while v2 answers in its
	// own C6 shape, which its published document declares for every 400 on
	// every route. Without the hook the one refusal that runs BEFORE v2's
	// own middleware would be the only v2 body its schema does not describe.
	OnInvalidLimit func(c *gin.Context, minPageSize, maxPageSize int)
}

// New creates Gin middleware for pagination with the provided Config.
func New(cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := getIntQueryParam(c, QueryParamOffset, cfg.DefaultPage)
		size := getIntQueryParam(c, QueryParamLimit, cfg.DefaultPageSize)

		if size < cfg.MinPageSize || size > cfg.MaxPageSize {
			if cfg.OnInvalidLimit != nil {
				cfg.OnInvalidLimit(c, cfg.MinPageSize, cfg.MaxPageSize)
				return
			}
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
