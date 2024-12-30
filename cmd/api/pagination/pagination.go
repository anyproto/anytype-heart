package pagination

import "github.com/gin-gonic/gin"

type Service[T any] interface {
	RespondWithPagination(c *gin.Context, statusCode int, data []T, total, offset, limit int, hasMore bool)
	Paginate(records []T, offset, limit int) ([]T, bool)
}

// RespondWithPagination returns a json response with the paginated data and corresponding metadata
func RespondWithPagination[T any](c *gin.Context, statusCode int, data []T, total, offset, limit int, hasMore bool) {
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

// Paginate paginates the given records based on the offset and limit
func Paginate[T any](records []T, offset, limit int) ([]T, bool) {
	total := len(records)
	start := offset
	end := offset + limit

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginated := records[start:end]
	hasMore := end < total
	return paginated, hasMore
}
