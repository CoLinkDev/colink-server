package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"colink-server/internal/pkg"
	"colink-server/internal/service"
)

type NoteAttachmentHandler struct {
	attachmentService *service.AttachmentService
}

func NewNoteAttachmentHandler(attachmentService *service.AttachmentService) *NoteAttachmentHandler {
	return &NoteAttachmentHandler{attachmentService: attachmentService}
}

func (h *NoteAttachmentHandler) Upload(c *gin.Context) {
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(15 * time.Minute))
	defer func() {
		_ = controller.SetReadDeadline(time.Time{})
	}()

	maxRequestBytes := h.attachmentService.MaxUploadRequestBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	form, err := c.Request.MultipartReader()
	if err != nil {
		writeError(c, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body"))
		return
	}
	result, err := h.attachmentService.Upload(userIDFromContext(c), form)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteAttachmentHandler) Get(c *gin.Context) {
	result, err := h.attachmentService.GetAttachment(userIDFromContext(c), c.Param("attachmentId"))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteAttachmentHandler) Delete(c *gin.Context) {
	if err := h.attachmentService.DeleteAttachment(userIDFromContext(c), c.Param("attachmentId")); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}

func (h *NoteAttachmentHandler) References(c *gin.Context) {
	result, err := h.attachmentService.ListReferences(
		userIDFromContext(c),
		c.Param("attachmentId"),
		c.Query("pageToken"),
		c.Query("limit"),
	)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteAttachmentHandler) Storage(c *gin.Context) {
	result, err := h.attachmentService.StorageUsage(userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

// Content streams the raw attachment body with ETag and single-range
// support. Errors fall back to the common JSON error envelope.
func (h *NoteAttachmentHandler) Content(c *gin.Context) {
	content, err := h.attachmentService.GetContent(userIDFromContext(c), c.Param("attachmentId"))
	if err != nil {
		writeError(c, err)
		return
	}

	attachment := content.Attachment
	etag := fmt.Sprintf(`"%s"`, attachment.SHA256)

	if match := c.GetHeader("If-None-Match"); match != "" && ifNoneMatchSatisfied(match, etag) {
		c.Header("ETag", etag)
		c.Status(http.StatusNotModified)
		return
	}

	file, err := openAttachmentFile(content.Path)
	if err != nil {
		writeError(c, err)
		return
	}
	defer func() {
		_ = file.Close()
	}()

	c.Header("ETag", etag)
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", attachment.MediaType)
	c.Header("Content-Disposition", contentDisposition(attachment.FileName))

	rangeHeader := c.GetHeader("Range")
	if rangeHeader == "" {
		c.Header("Content-Length", fmt.Sprintf("%d", attachment.Size))
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, file)
		return
	}

	start, end, ok := parseSingleByteRange(rangeHeader, attachment.Size)
	if !ok {
		c.Header("Content-Range", fmt.Sprintf("bytes */%d", attachment.Size))
		c.Status(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		writeError(c, pkg.InternalError(err))
		return
	}

	c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, attachment.Size))
	c.Header("Content-Length", fmt.Sprintf("%d", end-start+1))
	c.Status(http.StatusPartialContent)
	_, _ = io.CopyN(c.Writer, file, end-start+1)
}

// parseSingleByteRange parses a single `bytes=` range header value. Multiple
// ranges, other units, and unsatisfiable ranges are rejected.
func parseSingleByteRange(header string, size int64) (int64, int64, bool) {
	if size <= 0 {
		return 0, 0, false
	}

	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	if strings.Contains(spec, ",") {
		return 0, 0, false
	}

	dash := strings.Index(spec, "-")
	if dash < 0 {
		return 0, 0, false
	}
	startRaw := strings.TrimSpace(spec[:dash])
	endRaw := strings.TrimSpace(spec[dash+1:])
	if startRaw == "" && endRaw == "" {
		return 0, 0, false
	}

	var start, end int64
	if startRaw == "" {
		// Suffix range: last N bytes.
		suffix, err := parseInt64(endRaw)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix > size {
			suffix = size
		}
		start = size - suffix
		end = size - 1
	} else {
		value, err := parseInt64(startRaw)
		if err != nil || value < 0 {
			return 0, 0, false
		}
		start = value
		if start >= size {
			return 0, 0, false
		}
		if endRaw == "" {
			end = size - 1
		} else {
			value, err := parseInt64(endRaw)
			if err != nil || value < start {
				return 0, 0, false
			}
			end = value
			if end >= size {
				end = size - 1
			}
		}
	}

	return start, end, true
}

func ifNoneMatchSatisfied(header string, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || candidate == "W/"+etag {
			return true
		}
	}

	return false
}

func contentDisposition(fileName string) string {
	if fileName == "" {
		return "attachment"
	}
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, fileName)

	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", ascii, url.PathEscape(fileName))
}

func openAttachmentFile(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	return file, nil
}
