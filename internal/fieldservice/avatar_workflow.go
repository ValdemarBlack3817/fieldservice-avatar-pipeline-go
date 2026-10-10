package fieldservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const maxAvatarBytes = 8 << 20

type AvatarProcessor interface {
	Upload(context.Context, string, []byte) (string, error)
	SmartCrop(context.Context, string, []byte) (string, error)
	Resize(context.Context, string, []byte) (string, error)
	UpdateUserAvatar(context.Context, string, string, string) error
}

type AvatarWorkflow struct {
	infrai     AvatarProcessor
	downloader *http.Client
}

type AvatarRequest struct {
	UserID             string
	WorkOrderID        string
	DispatchStatus     string
	TechnicianFollowUp string
	Filename           string
	Image              []byte
}

type AvatarResult struct {
	WorkOrderID    string `json:"work_order_id"`
	DispatchStatus string `json:"dispatch_status"`
	AvatarURL      string `json:"avatar_url"`
	RequestID      string `json:"request_id"`
}

func NewAvatarWorkflow(infrai AvatarProcessor, downloader *http.Client) *AvatarWorkflow {
	if downloader == nil {
		downloader = &http.Client{}
	}
	return &AvatarWorkflow{infrai: infrai, downloader: downloader}
}

func (w *AvatarWorkflow) Process(ctx context.Context, in AvatarRequest) (AvatarResult, error) {
	if err := validateAvatarRequest(in); err != nil {
		return AvatarResult{}, err
	}
	requestID := stableRequestID(in)
	uploadedURL, err := w.infrai.Upload(ctx, in.Filename, in.Image)
	if err != nil {
		return AvatarResult{}, err
	}
	uploaded, err := w.download(ctx, uploadedURL)
	if err != nil {
		return AvatarResult{}, err
	}
	croppedURL, err := w.infrai.SmartCrop(ctx, "cropped-"+in.Filename, uploaded)
	if err != nil {
		return AvatarResult{}, err
	}
	cropped, err := w.download(ctx, croppedURL)
	if err != nil {
		return AvatarResult{}, err
	}
	avatarURL, err := w.infrai.Resize(ctx, "avatar-"+in.Filename, cropped)
	if err != nil {
		return AvatarResult{}, err
	}
	if err := w.infrai.UpdateUserAvatar(ctx, in.UserID, avatarURL, requestID); err != nil {
		return AvatarResult{}, err
	}
	return AvatarResult{WorkOrderID: in.WorkOrderID, DispatchStatus: in.DispatchStatus, AvatarURL: avatarURL, RequestID: requestID}, nil
}

func validateAvatarRequest(in AvatarRequest) error {
	if in.UserID == "" || in.WorkOrderID == "" || in.Filename == "" || len(in.Image) == 0 {
		return fmt.Errorf("user_id, work_order_id, photo, and filename are required")
	}
	validStatus := map[string]bool{"assigned": true, "en_route": true, "on_site": true, "completed": true}
	if !validStatus[in.DispatchStatus] {
		return fmt.Errorf("dispatch_status must be assigned, en_route, on_site, or completed")
	}
	if in.DispatchStatus == "completed" && in.TechnicianFollowUp == "" {
		return fmt.Errorf("technician_follow_up is required for completed work")
	}
	if len(in.Image) > maxAvatarBytes {
		return fmt.Errorf("photo exceeds 8 MiB")
	}
	return nil
}

func stableRequestID(in AvatarRequest) string {
	sum := sha256.Sum256(append([]byte(in.UserID+"\x00"+in.WorkOrderID+"\x00"), in.Image...))
	return hex.EncodeToString(sum[:16])
}

func (w *AvatarWorkflow) download(ctx context.Context, location string) ([]byte, error) {
	parsed, err := url.Parse(location)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid image location")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	res, err := w.downloader.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download pipeline image: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("download pipeline image status %d", res.StatusCode)
	}
	image, err := io.ReadAll(io.LimitReader(res.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(image) > maxAvatarBytes {
		return nil, fmt.Errorf("pipeline image exceeds 8 MiB")
	}
	return image, nil
}
