package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/workers"
)

// cov6xCapturingQueue is a workers.Queue that records Enqueue calls, or fails
// them. The queue is the true external boundary of the dispatch function.
type cov6xCapturingQueue struct {
	mu      sync.Mutex
	tasks   []*workers.Task
	opts    []workers.EnqueueOptions
	failure error
}

func (q *cov6xCapturingQueue) Enqueue(_ context.Context, task *workers.Task, opts ...workers.Option) (*workers.TaskInfo, error) {
	if q.failure != nil {
		return nil, q.failure
	}
	var o workers.EnqueueOptions
	for _, opt := range opts {
		opt(&o)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
	q.opts = append(q.opts, o)
	return &workers.TaskInfo{ID: "t1", Type: task.Type}, nil
}

func (q *cov6xCapturingQueue) EnqueueIn(ctx context.Context, _ time.Duration, task *workers.Task, opts ...workers.Option) (*workers.TaskInfo, error) {
	return q.Enqueue(ctx, task, opts...)
}

type cov6xCampaignFixture struct {
	h          *CommsHandlers
	svc        *comms.Service
	campaignID string
}

// cov6xNewCampaign builds a draft all-club campaign with `emails` household
// recipients, using real in-memory comms/members/groups services.
func cov6xNewCampaign(t *testing.T, emails ...string) cov6xCampaignFixture {
	t.Helper()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cov6xOrg, Roles: []string{"admin"}}
	msvc, gsvc, csvc := members.NewService(), groups.NewService(), comms.NewService()
	for i, e := range emails {
		if _, err := msvc.CreateHousehold(admin, members.CreateHouseholdInput{
			OrgID: cov6xOrg, HouseholdName: "Family" + string(rune('A'+i)), PrimaryEmail: e, Status: "active",
		}); err != nil {
			t.Fatalf("create household: %v", err)
		}
	}
	c, err := csvc.CreateCampaign(comms.CreateCampaignInput{
		OrgID: cov6xOrg, Name: "Welcome", Subject: "Hi", BodyHTML: "<p>hi</p>", BodyText: "hi",
		FromName: "Club", FromEmail: "club@example.com", Audience: "all", CreatedBy: admin.UserID,
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return cov6xCampaignFixture{h: NewCommsHandlers(csvc, msvc, gsvc), svc: csvc, campaignID: c.ID}
}

func (f cov6xCampaignFixture) claim(t *testing.T) CampaignSendPayload {
	t.Helper()
	claimed, err := f.svc.ClaimCampaign(context.Background(), cov6xOrg, f.campaignID, "u-admin")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return CampaignSendPayload{OrgID: cov6xOrg, CampaignID: f.campaignID, ActorID: "u-admin", ClaimedAtUnix: claimed.UpdatedAt.UnixNano()}
}

func (f cov6xCampaignFixture) status(t *testing.T) string {
	t.Helper()
	c, err := f.svc.GetCampaign(cov6xOrg, f.campaignID)
	if err != nil {
		t.Fatalf("get campaign: %v", err)
	}
	return c.Status
}

func TestCov6xRunCampaignSendRejectsBadPayloads(t *testing.T) {
	t.Parallel()
	f := cov6xNewCampaign(t, "a@example.com")

	for _, p := range []CampaignSendPayload{{}, {OrgID: cov6xOrg}, {CampaignID: f.campaignID}} {
		err := f.h.runCampaignSend(context.Background(), p)
		if err == nil || !strings.Contains(err.Error(), "missing org_id or campaign_id") {
			t.Errorf("payload %+v: want missing-id error, got %v", p, err)
		}
	}
	err := f.h.runCampaignSend(context.Background(), CampaignSendPayload{OrgID: cov6xOrg, CampaignID: "nope"})
	if !errors.Is(err, comms.ErrCampaignNotFound) || !strings.Contains(err.Error(), "load campaign") {
		t.Fatalf("unknown campaign: want wrapped ErrCampaignNotFound, got %v", err)
	}

	// A campaign that was never claimed is skipped, not sent.
	if err := f.h.runCampaignSend(context.Background(), CampaignSendPayload{OrgID: cov6xOrg, CampaignID: f.campaignID}); err != nil {
		t.Fatalf("unclaimed campaign should be skipped without error, got %v", err)
	}
	if got := f.status(t); got != "draft" {
		t.Fatalf("skipped campaign must stay draft, got %q", got)
	}
}

func TestCov6xRunCampaignSendSendsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	f := cov6xNewCampaign(t, "a@example.com", "b@example.com")
	p := f.claim(t)

	if err := f.h.runCampaignSend(context.Background(), p); err != nil {
		t.Fatalf("send: %v", err)
	}
	c, err := f.svc.GetCampaign(cov6xOrg, f.campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "sent" || c.SentCount != 2 || c.TotalRecipients != 2 {
		t.Fatalf("unexpected campaign after send: status=%s sent=%d total=%d", c.Status, c.SentCount, c.TotalRecipients)
	}
	// A redelivered job for a finished campaign is a no-op.
	if err := f.h.runCampaignSend(context.Background(), p); err != nil {
		t.Fatalf("redelivery should be a no-op, got %v", err)
	}
	if again, _ := f.svc.GetCampaign(cov6xOrg, f.campaignID); again.SentCount != 2 {
		t.Fatalf("redelivery changed sent count to %d", again.SentCount)
	}
}

func TestCov6xRunCampaignSendFailuresReleaseTheClaim(t *testing.T) {
	t.Parallel()

	t.Run("recipient resolution unavailable", func(t *testing.T) {
		t.Parallel()
		f := cov6xNewCampaign(t, "a@example.com")
		f.h.membersService = nil
		p := f.claim(t)
		err := f.h.runCampaignSend(context.Background(), p)
		if err == nil || !strings.Contains(err.Error(), "resolve recipients") {
			t.Fatalf("want resolve-recipients error, got %v", err)
		}
		if got := f.status(t); got != "draft" {
			t.Fatalf("claim must be released so the send can be retried, status=%q", got)
		}
	})

	t.Run("daily quota exceeded", func(t *testing.T) {
		t.Parallel()
		f := cov6xNewCampaign(t, "a@example.com", "b@example.com")
		f.svc.SetDailyEmailCap(1)
		p := f.claim(t)
		err := f.h.runCampaignSend(context.Background(), p)
		if !errors.Is(err, comms.ErrEmailQuotaExceeded) || !strings.HasPrefix(err.Error(), "campaign send: ") {
			t.Fatalf("want wrapped ErrEmailQuotaExceeded, got %v", err)
		}
		if got := f.status(t); got != "draft" {
			t.Fatalf("quota failure must return the campaign to draft, status=%q", got)
		}
	})
}

func TestCov6xCampaignSendJobHandler(t *testing.T) {
	t.Parallel()
	f := cov6xNewCampaign(t, "a@example.com")
	handler := f.h.CampaignSendJobHandler()

	if err := handler(context.Background(), &workers.Task{Type: TaskTypeCampaignSend, Payload: json.RawMessage(`{"org_id":`)}); err == nil ||
		!strings.Contains(err.Error(), "decode payload") {
		t.Fatalf("malformed payload: want decode error, got %v", err)
	}

	// The error from the run is returned so the queue retries the job.
	bad, _ := workers.NewTask(TaskTypeCampaignSend, CampaignSendPayload{OrgID: cov6xOrg})
	if err := handler(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "missing org_id or campaign_id") {
		t.Fatalf("invalid job should fail for retry, got %v", err)
	}

	task, err := workers.NewTask(TaskTypeCampaignSend, f.claim(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := f.status(t); got != "sent" {
		t.Fatalf("status = %q, want sent", got)
	}
}

func TestCov6xDispatchCampaignSendWithQueue(t *testing.T) {
	t.Parallel()
	f := cov6xNewCampaign(t, "a@example.com")
	q := &cov6xCapturingQueue{}
	f.h.SetJobQueue(q)
	p := CampaignSendPayload{OrgID: cov6xOrg, CampaignID: f.campaignID, ActorID: "u-admin", ClaimedAtUnix: 1234}

	if err := f.h.dispatchCampaignSend(context.Background(), p); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(q.tasks) != 1 || q.tasks[0].Type != TaskTypeCampaignSend {
		t.Fatalf("expected one %s task, got %+v", TaskTypeCampaignSend, q.tasks)
	}
	var got CampaignSendPayload
	if err := json.Unmarshal(q.tasks[0].Payload, &got); err != nil || got != p {
		t.Fatalf("payload round-trip = %+v (%v), want %+v", got, err, p)
	}
	if q.opts[0].Queue != workers.QueueEmail || q.opts[0].UniqueKey != "campaign-send:"+f.campaignID+":1234" {
		t.Fatalf("unexpected enqueue options: %+v", q.opts[0])
	}

	q.failure = errors.New("redis unavailable")
	err := f.h.dispatchCampaignSend(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "enqueue campaign send") || !errors.Is(err, q.failure) {
		t.Fatalf("enqueue failure should be wrapped, got %v", err)
	}
}

func TestCov6xDispatchCampaignSendWithoutQueueRunsInBackground(t *testing.T) {
	t.Parallel()
	f := cov6xNewCampaign(t, "a@example.com")
	p := f.claim(t)

	if err := f.h.dispatchCampaignSend(context.Background(), p); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// The send runs on a goroutine; poll with a deadline for its outcome.
	deadline := time.Now().Add(10 * time.Second)
	for f.status(t) == "sending" {
		if time.Now().After(deadline) {
			t.Fatal("background campaign send did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := f.status(t); got != "sent" {
		t.Fatalf("status = %q, want sent", got)
	}

	// A failing background send is logged, not surfaced: dispatch itself
	// still reports success and the goroutine ends.
	if err := f.h.dispatchCampaignSend(context.Background(), CampaignSendPayload{}); err != nil {
		t.Fatalf("dispatch of an invalid payload should still return nil, got %v", err)
	}
}
