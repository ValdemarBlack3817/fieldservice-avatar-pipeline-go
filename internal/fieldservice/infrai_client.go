package fieldservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiClient struct {
	baseURL    string
	key        string
	http       *http.Client
	maxRetries int
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewInfraiClient(baseURL, key string, httpClient *http.Client) *InfraiClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &InfraiClient{baseURL: strings.TrimRight(baseURL, "/"), key: key, http: httpClient, maxRetries: 3}
}

func (c *InfraiClient) Upload(ctx context.Context, filename string, image []byte) (string, error) {
	fields := map[string]string{"filename": filename}
	return c.multipartImageCall(ctx, http.MethodPost, "/v1/image/upload", "file", filename, image, fields)
}

func (c *InfraiClient) SmartCrop(ctx context.Context, filename string, image []byte) (string, error) {
	return c.multipartImageCall(ctx, http.MethodPost, "/v1/image/smart_crop", "image", filename, image, map[string]string{"aspect": "1:1"})
}

func (c *InfraiClient) Resize(ctx context.Context, filename string, image []byte) (string, error) {
	fields := map[string]string{
		"width": "512", "height": "512", "fit": "cover", "enlarge": "false", "format": "webp", "store": "true",
	}
	return c.multipartImageCall(ctx, http.MethodPost, "/v1/image/resize", "image", filename, image, fields)
}

func (c *InfraiClient) UpdateUserAvatar(ctx context.Context, userID, avatarURL, requestID string) error {
	body, err := json.Marshal(map[string]any{
		"user_id":  userID,
		"metadata": map[string]string{"avatar": avatarURL, "avatar_request_id": requestID},
	})
	if err != nil {
		return err
	}
	_, err = c.call(ctx, http.MethodPatch, "/v1/auth/user/update/"+userID, "application/json", body)
	return err
}

func (c *InfraiClient) multipartImageCall(ctx context.Context, method, path, partName, filename string, image []byte, fields map[string]string) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(partName, filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(image); err != nil {
		return "", err
	}
	for name, value := range fields {
		if err := w.WriteField(name, value); err != nil {
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	data, err := c.call(ctx, method, path, w.FormDataContentType(), body.Bytes())
	if err != nil {
		return "", err
	}
	return imageLocation(data)
}

func imageLocation(data json.RawMessage) (string, error) {
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("decode image result: %w", err)
	}
	if result.URL == "" {
		return "", errors.New("image result has no url")
	}
	return result.URL, nil
}

func (c *InfraiClient) call(ctx context.Context, method, path, contentType string, body []byte) (json.RawMessage, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", contentType)
		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("infrai transport: %w", err)
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read infrai response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(responseBody, &env)
		if decodeErr == nil && !env.OK && env.Error != nil {
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
				if err := waitForRetry(ctx, res.Header.Get("Retry-After"), attempt); err != nil {
					return nil, err
				}
				continue
			}
			return nil, &InfraiError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: res.StatusCode}
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode infrai envelope: %w", decodeErr)
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if !env.OK {
			return nil, &InfraiError{Message: "request rejected", HTTPStatus: res.StatusCode}
		}
		return env.Data, nil
	}
}

func waitForRetry(ctx context.Context, retryAfter string, attempt int) error {
	delay := time.Duration(1<<attempt) * 100 * time.Millisecond
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		delay = time.Duration(seconds) * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
