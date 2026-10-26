package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nikyjain/whatomate/internal/config"
	"github.com/nikyjain/whatomate/internal/models"
	"github.com/nikyjain/whatomate/pkg/whatsapp"
	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// leadGraphMock fakes the Graph lead fetch and the WhatsApp send endpoint.
type leadGraphMock struct {
	server        *httptest.Server
	fetchFailures int32 // number of initial lead fetches that return HTTP 500
	fetches       int32
	sends         int32
}

func newLeadGraphMock(t *testing.T) *leadGraphMock {
	m := &leadGraphMock{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/messages") {
			n := atomic.AddInt32(&m.sends, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []map[string]string{{"id": "wamid." + string(rune('A'+n))}},
			})
			return
		}
		if atomic.AddInt32(&m.fetches, 1) <= atomic.LoadInt32(&m.fetchFailures) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("unavailable"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "lead",
			"form_id": "form_1",
			"field_data": []map[string]interface{}{
				{"name": "full_name", "values": []string{"Asha Rao"}},
				{"name": "phone_number", "values": []string{"98765 43210"}},
			},
		})
	}))
	t.Cleanup(m.server.Close)
	return m
}

func newLeadProcessorTestApp(t *testing.T, mock *leadGraphMock) *App {
	t.Helper()
	db := testutil.SetupTestDB(t)
	rdb := testutil.SetupTestRedis(t)
	if rdb == nil {
		t.Skip("TEST_REDIS_URL not set, skipping test")
	}
	log := testutil.NopLogger()
	return &App{
		DB:         db,
		Redis:      rdb,
		Log:        log,
		Config:     &config.Config{},
		WhatsApp:   whatsapp.NewWithBaseURL(log, mock.server.URL),
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type leadFixture struct {
	org     *models.Organization
	source  *models.LeadSource
	mapping *models.LeadFormMapping
}

func createLeadFixture(t *testing.T, app *App, withTemplate bool) leadFixture {
	t.Helper()
	org := testutil.CreateTestOrganization(t, app.DB)
	source := &models.LeadSource{
		BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, Name: "Page",
		PageID: "page-" + uuid.NewString()[:12], AppSecret: "s", PageAccessToken: "t",
		APIVersion: "v21.0", DefaultCountryCode: "91", IsActive: true,
	}
	require.NoError(t, app.DB.Create(source).Error)

	mapping := &models.LeadFormMapping{
		BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, LeadSourceID: source.ID,
		FormID: "*", IsActive: true, TemplateParams: models.JSONB{}, HeaderParams: models.JSONB{},
		ButtonParams: models.JSONB{}, Tags: models.StringArray{}, PhoneField: "phone_number", NameField: "full_name",
	}
	if withTemplate {
		account := testutil.CreateTestWhatsAppAccount(t, app.DB, org.ID)
		tmpl := testutil.CreateTestTemplate(t, app.DB, org.ID, account.Name)
		mapping.TemplateID = &tmpl.ID
		mapping.WhatsAppAccount = account.Name
		mapping.TemplateParams = models.JSONB{"1": "{{lead.first_name}}"}
	}
	require.NoError(t, app.DB.Create(mapping).Error)
	return leadFixture{org: org, source: source, mapping: mapping}
}

func createQueuedLead(t *testing.T, app *App, f leadFixture, status string, fieldData models.JSONB) *models.Lead {
	t.Helper()
	now := time.Now()
	lead := &models.Lead{
		BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.org.ID, LeadSourceID: f.source.ID,
		LeadgenID: "lg-" + uuid.NewString()[:12], PageID: f.source.PageID, FormID: "form_1",
		FieldData: fieldData, Status: status, NextAttemptAt: &now, Attempts: 1, LeadCreatedAt: now,
	}
	require.NoError(t, app.DB.Create(lead).Error)
	return lead
}

func reloadLead(t *testing.T, app *App, id uuid.UUID) *models.Lead {
	t.Helper()
	var lead models.Lead
	require.NoError(t, app.DB.First(&lead, "id = ?", id).Error)
	return &lead
}

// A lead whose first Graph fetch failed must be fetched again on retry
// instead of being processed with empty answers.
func TestLeadProcessor_RetryRefetchesLeadDetails(t *testing.T) {
	mock := newLeadGraphMock(t)
	mock.fetchFailures = 1
	app := newLeadProcessorTestApp(t, mock)
	p := NewLeadProcessor(app, time.Second)
	f := createLeadFixture(t, app, false)
	lead := createQueuedLead(t, app, f, models.StatusLeadReceived, models.JSONB{})

	p.processLead(context.Background(), lead)
	afterFailure := reloadLead(t, app, lead.ID)
	require.Equal(t, models.StatusLeadRetry, afterFailure.Status)

	p.processLead(context.Background(), afterFailure)
	final := reloadLead(t, app, lead.ID)
	assert.Equal(t, models.StatusLeadStored, final.Status, "last_error=%s", final.LastError)
	assert.Equal(t, "919876543210", final.Phone)
	assert.Equal(t, int32(2), atomic.LoadInt32(&mock.fetches))
}

// Re-running a lead that was already sent (e.g. a stale worker after lease
// expiry) must never produce a second WhatsApp message.
func TestLeadProcessor_NoDoubleSend(t *testing.T) {
	mock := newLeadGraphMock(t)
	app := newLeadProcessorTestApp(t, mock)
	p := NewLeadProcessor(app, time.Second)
	f := createLeadFixture(t, app, true)
	answers := models.JSONB{"full_name": "Asha Rao", "phone_number": "9876543210"}
	lead := createQueuedLead(t, app, f, models.StatusLeadFetched, answers)
	stale := *lead // copy held by a second worker that claimed the same row

	p.processLead(context.Background(), lead)
	sent := reloadLead(t, app, lead.ID)
	require.Equal(t, models.StatusLeadSent, sent.Status, "last_error=%s", sent.LastError)
	require.NotNil(t, sent.MessageID)
	require.Equal(t, int32(1), atomic.LoadInt32(&mock.sends))

	// Stale in-memory copy (status fetched, no message id) is refused.
	p.processLead(context.Background(), &stale)
	assert.Equal(t, int32(1), atomic.LoadInt32(&mock.sends))
	assert.Equal(t, models.StatusLeadSent, reloadLead(t, app, lead.ID).Status)

	// Manual retry of a delivered lead re-links the message instead of resending.
	require.NoError(t, app.requeueLead(sent, false))
	requeued := reloadLead(t, app, lead.ID)
	requeued.MessageID = sent.MessageID
	p.processLead(context.Background(), requeued)
	assert.Equal(t, int32(1), atomic.LoadInt32(&mock.sends))
	assert.Equal(t, models.StatusLeadSent, reloadLead(t, app, lead.ID).Status)
}

// processBatch claims due leads but never re-claims one that is mid-send.
func TestLeadProcessor_BatchClaimSkipsSending(t *testing.T) {
	mock := newLeadGraphMock(t)
	app := newLeadProcessorTestApp(t, mock)
	// Isolate from leads left behind by other tests in the shared database.
	require.NoError(t, app.DB.Exec("UPDATE leads SET deleted_at = NOW() WHERE deleted_at IS NULL").Error)
	p := NewLeadProcessor(app, time.Second)
	f := createLeadFixture(t, app, false)
	due := createQueuedLead(t, app, f, models.StatusLeadFetched, models.JSONB{"phone_number": "9876543210"})
	inFlight := createQueuedLead(t, app, f, models.StatusLeadSending, models.JSONB{"phone_number": "9876543210"})

	p.processBatch(context.Background())

	assert.Equal(t, models.StatusLeadStored, reloadLead(t, app, due.ID).Status)
	stuck := reloadLead(t, app, inFlight.ID)
	assert.Equal(t, models.StatusLeadSending, stuck.Status)
	assert.Equal(t, 1, stuck.Attempts, "a sending lead must not be claimed")
}

// Mappings of another organization must never be applied to a lead.
func TestLeadProcessor_MappingLookupIsOrgScoped(t *testing.T) {
	mock := newLeadGraphMock(t)
	app := newLeadProcessorTestApp(t, mock)
	p := NewLeadProcessor(app, time.Second)
	f := createLeadFixture(t, app, false)
	other := testutil.CreateTestOrganization(t, app.DB)
	// Move the only mapping to a different org while keeping the source link.
	require.NoError(t, app.DB.Model(&models.LeadFormMapping{}).Where("id = ?", f.mapping.ID).
		Update("organization_id", other.ID).Error)

	lead := createQueuedLead(t, app, f, models.StatusLeadFetched, models.JSONB{"phone_number": "9876543210"})
	p.processLead(context.Background(), lead)
	assert.Equal(t, models.StatusLeadSkippedNoMapping, reloadLead(t, app, lead.ID).Status)
}
