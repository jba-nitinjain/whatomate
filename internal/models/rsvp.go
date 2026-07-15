package models

import (
	"time"

	"github.com/google/uuid"
)

type RSVPEventStatus string

const (
	RSVPEventStatusDraft  RSVPEventStatus = "draft"
	RSVPEventStatusActive RSVPEventStatus = "active"
	RSVPEventStatusClosed RSVPEventStatus = "closed"
)

type RSVPAttendance string

const (
	RSVPAttendancePending RSVPAttendance = "pending"
	RSVPAttendanceYes     RSVPAttendance = "yes"
	RSVPAttendanceNo      RSVPAttendance = "no"
	RSVPAttendanceMaybe   RSVPAttendance = "maybe"
)

type RSVPAccessMode string

const (
	RSVPAccessModeGuestList   RSVPAccessMode = "guest_list"
	RSVPAccessModeOpenKeyword RSVPAccessMode = "open_keyword"
)

type RSVPGuestSource string

const (
	RSVPGuestSourceContactSelection  RSVPGuestSource = "contact_selection"
	RSVPGuestSourceSpreadsheetImport RSVPGuestSource = "spreadsheet_import"
	RSVPGuestSourceOpenKeyword       RSVPGuestSource = "open_keyword"
	RSVPGuestSourceAPI               RSVPGuestSource = "api"
	RSVPGuestSourceLegacy            RSVPGuestSource = "legacy"
)

type RSVPReminderScheduleStatus string

const (
	RSVPReminderSchedulePending             RSVPReminderScheduleStatus = "pending"
	RSVPReminderScheduleProcessing          RSVPReminderScheduleStatus = "processing"
	RSVPReminderScheduleCompleted           RSVPReminderScheduleStatus = "completed"
	RSVPReminderScheduleCompletedWithErrors RSVPReminderScheduleStatus = "completed_with_errors"
	RSVPReminderScheduleCancelled           RSVPReminderScheduleStatus = "cancelled"
)

type RSVPReminderDeliveryType string
type RSVPReminderDeliveryStatus string

const (
	RSVPReminderDeliveryManual    RSVPReminderDeliveryType   = "manual"
	RSVPReminderDeliveryScheduled RSVPReminderDeliveryType   = "scheduled"
	RSVPReminderDeliveryQueued    RSVPReminderDeliveryStatus = "queued"
	RSVPReminderDeliverySent      RSVPReminderDeliveryStatus = "sent"
	RSVPReminderDeliveryFailed    RSVPReminderDeliveryStatus = "failed"
	RSVPReminderDeliverySkipped   RSVPReminderDeliveryStatus = "skipped"
)

// RSVPEvent is an org-scoped RSVP that links a chatbot flow to tallied responses.
type RSVPEvent struct {
	BaseModel
	OrganizationID uuid.UUID       `gorm:"type:uuid;index;not null" json:"organization_id"`
	Name           string          `gorm:"size:255;not null" json:"name"`
	Description    string          `gorm:"type:text" json:"description"`
	EventDate      *time.Time      `json:"event_date,omitempty"`
	RSVPCloseAt    *time.Time      `json:"rsvp_close_at,omitempty"`
	Status         RSVPEventStatus `gorm:"size:20;default:'draft'" json:"status"`
	// The database default preserves the behavior of rows created before eligibility
	// modes existed. The API explicitly defaults newly-created events to guest_list.
	AccessMode        RSVPAccessMode `gorm:"size:30;not null;default:'open_keyword';index" json:"access_mode"`
	NotInvitedMessage string         `gorm:"type:text" json:"not_invited_message"`

	// WhatsApp account (by Name) used to send invites/reminders.
	WhatsAppAccount string `gorm:"size:100;index" json:"whatsapp_account"`

	// Entry: linked flow + keyword. Keyword must be unique among active events per org.
	FlowID  *uuid.UUID `gorm:"type:uuid" json:"flow_id,omitempty"`
	Keyword string     `gorm:"size:100;index" json:"keyword"`

	// Attendance mapping: which SessionData key holds the attendance answer,
	// and how its raw value maps to yes/no/maybe.
	AttendanceField string `gorm:"size:100;default:'attendance'" json:"attendance_field"`
	AttendanceMap   JSONB  `gorm:"type:jsonb;default:'{}'" json:"attendance_map"`

	// Duplicate handling: SpouseMobileField is the answer key holding a spouse's
	// mobile; a new responder whose number already responded (as responder or as a
	// recorded spouse) is turned away with DuplicateMessage instead of re-asked.
	SpouseMobileField string `gorm:"size:100" json:"spouse_mobile_field"`
	DuplicateMessage  string `gorm:"type:text" json:"duplicate_message"`

	// Invite template (optional, for campaign/keyword invite send).
	TemplateID *uuid.UUID `gorm:"type:uuid" json:"template_id,omitempty"`

	// Reminders.
	ReminderEnabled    bool       `gorm:"default:false" json:"reminder_enabled"`
	ReminderAt         *time.Time `json:"reminder_at,omitempty"`
	ReminderTemplateID *uuid.UUID `gorm:"type:uuid" json:"reminder_template_id,omitempty"`
	ReminderSentAt     *time.Time `json:"reminder_sent_at,omitempty"`

	CreatedBy uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`

	Organization      *Organization          `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Responses         []RSVPResponse         `gorm:"foreignKey:RSVPEventID" json:"responses,omitempty"`
	ReminderSchedules []RSVPReminderSchedule `gorm:"foreignKey:RSVPEventID" json:"reminder_schedules,omitempty"`
}

func (RSVPEvent) TableName() string { return "rsvp_events" }

// RSVPResponse is one guest's answer set for an event. Unique per (event, contact).
type RSVPResponse struct {
	BaseModel
	RSVPEventID     uuid.UUID       `gorm:"type:uuid;index:idx_rsvp_event_contact,unique;not null" json:"rsvp_event_id"`
	OrganizationID  uuid.UUID       `gorm:"type:uuid;index;not null" json:"organization_id"`
	ContactID       uuid.UUID       `gorm:"type:uuid;index:idx_rsvp_event_contact,unique;not null" json:"contact_id"`
	PhoneNumber     string          `gorm:"size:50;not null" json:"phone_number"`
	Attendance      RSVPAttendance  `gorm:"size:20;default:'pending'" json:"attendance"`
	Answers         JSONB           `gorm:"type:jsonb;default:'{}'" json:"answers"`
	Notes           string          `gorm:"type:text" json:"notes"`
	RespondedAt     *time.Time      `json:"responded_at,omitempty"`
	RepromptedAt    *time.Time      `json:"reprompted_at,omitempty"` // last time the RSVP flow was re-sent to this guest
	Source          RSVPGuestSource `gorm:"size:30;not null;default:'legacy';index" json:"source"`
	InviteSentAt    *time.Time      `json:"invite_sent_at,omitempty"`
	InviteMessageID string          `gorm:"size:255" json:"invite_message_id"`
	RSVPStartedAt   *time.Time      `json:"rsvp_started_at,omitempty"`

	Event   *RSVPEvent `gorm:"foreignKey:RSVPEventID" json:"event,omitempty"`
	Contact *Contact   `gorm:"foreignKey:ContactID" json:"contact,omitempty"`
}

func (RSVPResponse) TableName() string { return "rsvp_responses" }

type RSVPReminderSchedule struct {
	BaseModel
	RSVPEventID    uuid.UUID                  `gorm:"type:uuid;index;not null" json:"rsvp_event_id"`
	OrganizationID uuid.UUID                  `gorm:"type:uuid;index;not null" json:"organization_id"`
	ScheduledAt    time.Time                  `gorm:"index;not null" json:"scheduled_at"`
	TemplateID     uuid.UUID                  `gorm:"type:uuid;not null" json:"template_id"`
	TemplateParams JSONB                      `gorm:"type:jsonb;default:'{}'" json:"template_params"`
	Status         RSVPReminderScheduleStatus `gorm:"size:40;index;not null;default:'pending'" json:"status"`
	SentCount      int                        `json:"sent_count"`
	FailedCount    int                        `json:"failed_count"`
	ProcessedAt    *time.Time                 `json:"processed_at,omitempty"`
	CreatedBy      uuid.UUID                  `gorm:"type:uuid;not null" json:"created_by"`
	CampaignID     *uuid.UUID                 `gorm:"type:uuid;index" json:"campaign_id,omitempty"`
}

func (RSVPReminderSchedule) TableName() string { return "rsvp_reminder_schedules" }

type RSVPReminderDelivery struct {
	BaseModel
	RSVPEventID         uuid.UUID                  `gorm:"type:uuid;index;not null" json:"rsvp_event_id"`
	OrganizationID      uuid.UUID                  `gorm:"type:uuid;index;not null" json:"organization_id"`
	RSVPResponseID      uuid.UUID                  `gorm:"type:uuid;index;not null;uniqueIndex:idx_rsvp_schedule_response" json:"rsvp_response_id"`
	ScheduleID          *uuid.UUID                 `gorm:"type:uuid;index;uniqueIndex:idx_rsvp_schedule_response" json:"schedule_id,omitempty"`
	DeliveryType        RSVPReminderDeliveryType   `gorm:"size:20;not null" json:"delivery_type"`
	Status              RSVPReminderDeliveryStatus `gorm:"size:20;not null" json:"status"`
	MessageID           string                     `gorm:"size:255" json:"message_id"`
	ErrorMessage        string                     `gorm:"type:text" json:"error_message"`
	AttemptedAt         time.Time                  `gorm:"not null" json:"attempted_at"`
	InitiatedBy         *uuid.UUID                 `gorm:"type:uuid" json:"initiated_by,omitempty"`
	CampaignID          *uuid.UUID                 `gorm:"type:uuid;index" json:"campaign_id,omitempty"`
	CampaignRecipientID *uuid.UUID                 `gorm:"type:uuid;index;uniqueIndex" json:"campaign_recipient_id,omitempty"`
}

func (RSVPReminderDelivery) TableName() string { return "rsvp_reminder_deliveries" }
