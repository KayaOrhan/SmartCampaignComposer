package campaign

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type ApprovalPreview struct {
	Preview
	Revision            int64   `json:"revision"`
	ApprovalFingerprint *string `json:"approval_fingerprint"`
}

type ApprovalPreviewChanged struct{ Preview ApprovalPreview }

func (e *ApprovalPreviewChanged) Error() string { return "approval_preview_changed" }

func ValidateApprovalFingerprint(value string) error {
	if len(value) != 64 || strings.ToLower(value) != value {
		return segment.Invalid("invalid_approval_fingerprint", "approval_fingerprint")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return segment.Invalid("invalid_approval_fingerprint", "approval_fingerprint")
	}
	return nil
}

type DeliveryCounts struct {
	Total    int64 `json:"total"`
	Pending  int64 `json:"pending"`
	InFlight int64 `json:"in_flight"`
	Sent     int64 `json:"sent"`
	Dead     int64 `json:"dead"`
	Skipped  int64 `json:"skipped"`
}

type DeliveryStatus struct {
	CampaignID       int64                     `json:"campaign_id"`
	Revision         int64                     `json:"revision"`
	Status           string                    `json:"status"`
	ApprovedAt       *time.Time                `json:"approved_at"`
	ApprovedRevision *int64                    `json:"approved_revision"`
	Counts           DeliveryCounts            `json:"counts"`
	ByChannel        map[string]DeliveryCounts `json:"by_channel"`
}
