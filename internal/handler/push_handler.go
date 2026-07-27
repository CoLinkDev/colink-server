package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"colink-server/internal/pkg"
	"colink-server/internal/service"
	"colink-server/internal/ws"
)

type PushHandler struct {
	wsService *service.WsService
}

type pushRequest struct {
	DeviceKey  *string  `json:"device_key"`
	DeviceKeys []string `json:"device_keys"`
	ws.PushNotificationPayload
}

type pushResponse struct {
	Code      int                `json:"code"`
	Message   string             `json:"message"`
	Timestamp int64              `json:"timestamp"`
	Results   []pushDeviceResult `json:"results,omitempty"`
}

type pushDeviceResult struct {
	DeviceKey string `json:"device_key"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
}

func NewPushHandler(wsService *service.WsService) *PushHandler {
	return &PushHandler{wsService: wsService}
}

func (h *PushHandler) Unauthorized(c *gin.Context) {
	writePushResponse(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized", nil)
}

func (h *PushHandler) Send(c *gin.Context) {
	request, targets, batch, err := parsePushRequest(c)
	if err != nil {
		writePushError(c, err)
		return
	}

	if batch {
		userID := userIDFromContext(c)
		results := make([]pushDeviceResult, len(targets))
		var group sync.WaitGroup
		for index, target := range targets {
			group.Add(1)
			go func(index int, target string) {
				defer group.Done()
				result := pushDeviceResult{
				DeviceKey: target,
				Code:      http.StatusOK,
				Message:   "success",
				}
				if err := h.wsService.DeliverPush(userID, target, request.PushNotificationPayload); err != nil {
					result.Code, result.Message = pushErrorCodeAndMessage(err)
			}
				results[index] = result
			}(index, target)
		}
		group.Wait()
		writePushResponse(c, http.StatusOK, http.StatusOK, "success", results)
		return
	}

	if err := h.wsService.DeliverPush(userIDFromContext(c), targets[0], request.PushNotificationPayload); err != nil {
		writePushError(c, err)
		return
	}
	writePushResponse(c, http.StatusOK, http.StatusOK, "success", nil)
}

func parsePushRequest(c *gin.Context) (pushRequest, []string, bool, error) {
	var request pushRequest
	isJSON := strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json")
	if isJSON {
		decoder := json.NewDecoder(c.Request.Body)
		if err := decoder.Decode(&request); err != nil {
			return request, nil, false, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
		}
		if err := ensureSingleJSONValue(decoder); err != nil {
			return request, nil, false, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
		}
		if err := applyPushValues(c.Request.URL.Query(), &request); err != nil {
			return request, nil, false, err
		}
	} else {
		if err := c.Request.ParseForm(); err != nil {
			return request, nil, false, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
		}
		if c.Request.PostForm.Has("device_keys") || c.Request.URL.Query().Has("device_keys") {
			return request, nil, false, invalidPushParameter()
		}
		if err := applyPushValues(c.Request.URL.Query(), &request); err != nil {
			return request, nil, false, err
		}
		if err := applyPushValues(c.Request.PostForm, &request); err != nil {
			return request, nil, false, err
		}
	}

	if err := validatePushPayload(request.PushNotificationPayload); err != nil {
		return request, nil, false, err
	}

	if path := strings.Trim(c.Param("path"), "/"); path != "" {
		deviceID, err := applyPathPushValues(strings.Split(path, "/"), &request)
		if err != nil {
			return request, nil, false, err
		}
		return request, []string{deviceID}, false, nil
	}
	if !isJSON {
		return request, nil, false, invalidPushParameter()
	}
	if len(request.DeviceKeys) > 0 {
		if request.DeviceKey != nil || len(request.DeviceKeys) == 0 {
			return request, nil, false, invalidPushParameter()
		}
		for _, deviceID := range request.DeviceKeys {
			if strings.TrimSpace(deviceID) == "" {
				return request, nil, false, invalidPushParameter()
			}
		}
		return request, request.DeviceKeys, true, nil
	}
	if request.DeviceKey == nil || strings.TrimSpace(*request.DeviceKey) == "" {
		return request, nil, false, invalidPushParameter()
	}
	return request, []string{*request.DeviceKey}, false, nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func applyPathPushValues(segments []string, request *pushRequest) (string, error) {
	if len(segments) == 0 || len(segments) > 4 || segments[0] == "" {
		return "", invalidPushParameter()
	}

	if len(segments) >= 2 {
		body := segments[len(segments)-1]
		request.Body = &body
	}
	if len(segments) >= 3 {
		title := segments[1]
		request.Title = &title
	}
	if len(segments) == 4 {
		subtitle := segments[2]
		request.Subtitle = &subtitle
	}
	return segments[0], nil
}

func applyPushValues(values url.Values, request *pushRequest) error {
	applyPushString(values, "device_key", &request.DeviceKey)
	applyPushString(values, "title", &request.Title)
	applyPushString(values, "subtitle", &request.Subtitle)
	applyPushString(values, "body", &request.Body)
	applyPushString(values, "markdown", &request.Markdown)
	applyPushString(values, "level", &request.Level)
	applyPushString(values, "sound", &request.Sound)
	applyPushString(values, "icon", &request.Icon)
	applyPushString(values, "image", &request.Image)
	applyPushString(values, "group", &request.Group)
	applyPushString(values, "url", &request.URL)
	applyPushString(values, "copy", &request.Copy)
	applyPushString(values, "id", &request.PushID)
	applyPushString(values, "action", &request.Action)
	applyPushString(values, "ciphertext", &request.Ciphertext)

	for _, field := range []struct {
		name   string
		target **int
	}{
		{"volume", &request.Volume},
		{"badge", &request.Badge},
		{"ttl", &request.TTL},
	} {
		if err := applyPushInt(values, field.name, field.target); err != nil {
			return err
		}
	}
	for _, field := range []struct {
		name   string
		target **bool
	}{
		{"autoCopy", &request.AutoCopy},
		{"call", &request.Call},
		{"isArchive", &request.IsArchive},
		{"delete", &request.Delete},
	} {
		if err := applyPushBool(values, field.name, field.target); err != nil {
			return err
		}
	}
	return nil
}

func applyPushString(values url.Values, name string, target **string) {
	if !values.Has(name) {
		return
	}
	value := values.Get(name)
	*target = &value
}

func applyPushInt(values url.Values, name string, target **int) error {
	if !values.Has(name) {
		return nil
	}
	value, err := strconv.Atoi(values.Get(name))
	if err != nil {
		return invalidPushParameter()
	}
	*target = &value
	return nil
}

func applyPushBool(values url.Values, name string, target **bool) error {
	if !values.Has(name) {
		return nil
	}
	value, err := parsePushBool(values.Get(name))
	if err != nil {
		return invalidPushParameter()
	}
	*target = &value
	return nil
}

func parsePushBool(value string) (bool, error) {
	switch value {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return strconv.ParseBool(value)
	}
}

func validatePushPayload(payload ws.PushNotificationPayload) error {
	if payload.Level != nil {
		switch *payload.Level {
		case "active", "timeSensitive", "passive", "critical":
		default:
			return invalidPushParameter()
		}
	}
	if payload.Volume != nil && (*payload.Volume < 0 || *payload.Volume > 10) {
		return invalidPushParameter()
	}
	if payload.TTL != nil && *payload.TTL < 0 {
		return invalidPushParameter()
	}
	if payload.Action != nil && *payload.Action != "alert" {
		return invalidPushParameter()
	}
	if payload.Delete != nil && *payload.Delete && (payload.PushID == nil || strings.TrimSpace(*payload.PushID) == "") {
		return invalidPushParameter()
	}
	return nil
}

func invalidPushParameter() error {
	return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
}

func writePushError(c *gin.Context, err error) {
	code, message := pushErrorCodeAndMessage(err)
	status := http.StatusInternalServerError
	if code == pkg.CodeUnauthorized {
		status = http.StatusUnauthorized
	} else if code == pkg.CodeInvalidRequestBody || code == pkg.CodeInvalidParameter {
		status = http.StatusBadRequest
	} else if code == pkg.CodeDeviceNotFound || code == pkg.CodePushDeviceOffline || code == pkg.CodePushNotSupported || code == pkg.CodePushTimeout {
		status = http.StatusOK
	}
	writePushResponse(c, status, code, message, nil)
}

func pushErrorCodeAndMessage(err error) (int, string) {
	var appErr *pkg.AppError
	if errors.As(err, &appErr) {
		return appErr.Code, appErr.Message
	}
	return pkg.CodeInternalError, "internal error"
}

func writePushResponse(c *gin.Context, status int, code int, message string, results []pushDeviceResult) {
	c.JSON(status, pushResponse{
		Code:      code,
		Message:   message,
		Timestamp: time.Now().UTC().UnixMilli(),
		Results:   results,
	})
}
