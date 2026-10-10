package fieldservice

import (
	"context"
	"testing"
)

type recordingProcessor struct{ calls int }

func (p *recordingProcessor) Upload(context.Context, string, []byte) (string, error) {
	p.calls++
	return "https://example.test/upload", nil
}
func (p *recordingProcessor) SmartCrop(context.Context, string, []byte) (string, error) {
	p.calls++
	return "https://example.test/crop", nil
}
func (p *recordingProcessor) Resize(context.Context, string, []byte) (string, error) {
	p.calls++
	return "https://example.test/avatar", nil
}
func (p *recordingProcessor) UpdateUserAvatar(context.Context, string, string, string) error {
	p.calls++
	return nil
}

func TestCompletedWorkRequiresTechnicianFollowUp(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		followUp string
		wantErr  bool
	}{
		{name: "active visit", status: "on_site", wantErr: false},
		{name: "completed with note", status: "completed", followUp: "filter replaced", wantErr: false},
		{name: "completed without note", status: "completed", wantErr: true},
		{name: "unknown status", status: "queued", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := AvatarRequest{UserID: "tech-42", WorkOrderID: "wo-1048", DispatchStatus: tt.status, TechnicianFollowUp: tt.followUp, Filename: "profile.jpg", Image: []byte("photo")}
			err := validateAvatarRequest(in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAvatarRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRejectedWorkDoesNotCallInfrai(t *testing.T) {
	processor := &recordingProcessor{}
	w := NewAvatarWorkflow(processor, nil)
	_, err := w.Process(context.Background(), AvatarRequest{UserID: "tech-42", WorkOrderID: "wo-1048", DispatchStatus: "completed", Filename: "profile.jpg", Image: []byte("photo")})
	if err == nil {
		t.Fatal("Process() expected validation error")
	}
	if processor.calls != 0 {
		t.Fatalf("Infrai calls = %d, want 0", processor.calls)
	}
}
