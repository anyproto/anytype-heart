package v2handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// DownloadFileHandler streams an authorized file or icon.
//
//	@Summary		Download file content
//	@Description	Returns a file or icon from this space. Pass a file id or the icon_image value from a space or member. Images support width variants. Range and conditional requests are supported.
//	@Id				download_file
//	@Tags			Files
//	@Produce		application/octet-stream
//	@Param			space_id		path		string			true	"Space id"
//	@Param			file_id			path		string			true	"File or icon id"
//	@Param			width			query		int				false	"Image variant width; zero selects the original"	minimum(0)
//	@Param			Range			header		string			false	"Byte range"
//	@Param			If-None-Match	header		string			false	"Previously received ETag"
//	@Param			If-Range		header		string			false	"ETag or modification date for a range request"
//	@Success		200				{file}		binary			"File contents; Content-Type matches the stored media type"
//	@Success		206				{file}		binary			"Requested byte range"
//	@Success		304				{string}	string			"Not modified; no response body"
//	@Failure		404				{object}	v2model.Error	"File or icon not found in this space"
//	@Failure		412				{object}	v2model.Error	"Request precondition failed"
//	@Failure		416				{object}	v2model.Error	"Invalid or unsatisfiable byte range"
//	@Failure		500				{object}	v2model.Error	"File content could not be read"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/files/{file_id}/content [get]
func DownloadFileHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		width := 0
		if raw, present := c.GetQuery("width"); present {
			var err error
			width, err = strconv.Atoi(raw)
			if err != nil || width < 0 {
				RespondError(c, v2model.ValidationFailed("width must be a non-negative integer",
					v2model.Issue{Path: "width", Message: "use zero for the original image or a positive pixel width"}))
				return
			}
		}
		content, err := s.GetFileContent(c.Request.Context(), c.Param("space_id"), c.Param("file_id"), width)
		if err != nil {
			RespondError(c, err)
			return
		}
		if content.MimeType != "" {
			c.Header("Content-Type", content.MimeType)
		}
		// Revalidate authorization even when a client has cached the bytes.
		c.Header("Cache-Control", "private, no-cache")
		c.Header("ETag", content.ETag)
		c.Header("X-Content-Type-Options", "nosniff")
		var modTime time.Time
		if content.ModTime > 0 {
			modTime = time.Unix(content.ModTime, 0)
		}
		writer := &fileResponseWriter{ResponseWriter: c.Writer}
		http.ServeContent(writer, c.Request, content.Name, modTime, content.Reader)
		if writer.errorStatus != 0 {
			c.Writer.Header().Del("Content-Type")
			c.Writer.Header().Del("Content-Length")
			code := v2model.CodeValidationFailed
			if writer.errorStatus >= http.StatusInternalServerError {
				code = v2model.CodeInternalError
			}
			RespondError(c, v2model.NewError(writer.errorStatus, code, http.StatusText(writer.errorStatus)))
		}
	}
}

// HeadFileHandler returns the same headers and performs the same access checks.
//
//	@Summary		Read file content headers
//	@Description	Returns the file or icon headers without a response body. Accepts the same ids as file download.
//	@Id				head_file
//	@Tags			Files
//	@Param			space_id	path	string	true	"Space id"
//	@Param			file_id		path	string	true	"File or icon id"
//	@Param			width		query	int		false	"Image variant width; zero selects the original"	minimum(0)
//	@Success		200			{string}	string			"File headers; no response body"
//	@Success		304			{string}	string			"Not modified; no response body"
//	@Failure		404			{object}	v2model.Error	"File or icon not found in this space"
//	@Failure		412			{object}	v2model.Error	"Request precondition failed"
//	@Failure		500			{object}	v2model.Error	"File content could not be read"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/files/{file_id}/content [head]
func HeadFileHandler(s *v2service.Service) gin.HandlerFunc {
	return DownloadFileHandler(s)
}

// Let ServeContent handle ranges and validators, translating its plain-text
// errors before committing the response. Successful bytes stream directly.
type fileResponseWriter struct {
	http.ResponseWriter
	errorStatus int
}

func (w *fileResponseWriter) WriteHeader(status int) {
	if status >= http.StatusBadRequest {
		w.errorStatus = status
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *fileResponseWriter) Write(p []byte) (int, error) {
	if w.errorStatus != 0 {
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}
