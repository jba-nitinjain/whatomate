package handlers

import "time"

const (
	chatRepairActionMove          = "move"
	chatRepairActionMergeRequired = "merge_required"
	chatRepairActionConflict      = "conflict"
)

type chatRepairBaseRow struct {
	ContactID            string     `gorm:"column:contact_id"`
	CurrentOrgID         string     `gorm:"column:current_org_id"`
	PhoneNumber          string     `gorm:"column:phone_number"`
	ProfileName          string     `gorm:"column:profile_name"`
	CurrentAccount       string     `gorm:"column:current_account"`
	TargetOrgCount       int64      `gorm:"column:target_org_count"`
	TargetAccountCount   int64      `gorm:"column:target_account_count"`
	TargetOrgID          string     `gorm:"column:target_org_id"`
	TargetAccount        string     `gorm:"column:target_account"`
	AffectedMessageCount int64      `gorm:"column:affected_message_count"`
	LastMessageAt        *time.Time `gorm:"column:last_message_at"`
	SamplePhoneNumberID  string     `gorm:"column:sample_phone_number_id"`
}

type ChatRepairCandidate struct {
	ContactID            string     `json:"contact_id"`
	PhoneNumber          string     `json:"phone_number"`
	ProfileName          string     `json:"profile_name"`
	CurrentOrgID         string     `json:"current_org_id"`
	CurrentOrgName       string     `json:"current_org_name,omitempty"`
	CurrentAccount       string     `json:"current_account"`
	TargetOrgID          string     `json:"target_org_id"`
	TargetOrgName        string     `json:"target_org_name,omitempty"`
	TargetAccount        string     `json:"target_account"`
	Action               string     `json:"action"`
	Reason               string     `json:"reason"`
	AffectedMessageCount int64      `json:"affected_message_count"`
	LastMessageAt        *time.Time `json:"last_message_at,omitempty"`
	PhoneNumberID        string     `json:"phone_number_id"`
	TargetContactID      string     `json:"target_contact_id,omitempty"`
}

type ChatRepairSummary struct {
	ScannedContacts          int64 `json:"scanned_contacts"`
	AffectedExternalMessages int64 `json:"affected_external_messages"`
	MoveCandidates           int64 `json:"move_candidates"`
	MergeRequiredCandidates  int64 `json:"merge_required_candidates"`
	ConflictCandidates       int64 `json:"conflict_candidates"`
	AutoFixableCandidates    int64 `json:"auto_fixable_candidates"`
}

type ChatRepairApplyRequest struct {
	ContactIDs []string `json:"contact_ids"`
}

type ChatRepairApplyResult struct {
	ProcessedCandidates int64 `json:"processed_candidates"`
	UpdatedContacts     int64 `json:"updated_contacts"`
	UpdatedMessages     int64 `json:"updated_messages"`
	SkippedCandidates   int64 `json:"skipped_candidates"`
}
