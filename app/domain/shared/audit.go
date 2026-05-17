package shared

import (
	"context"
	"sync"
	"time"
)

// AuditEvent is the handler-facing shape — what the application code
// emits via AuditCollector.Record. The middleware enriches it with
// actor + IP from context and persists it as an AuditRecord.
type AuditEvent struct {
	Action     string         `json:"action"`
	TargetType string         `json:"target_type,omitempty"`
	TargetID   string         `json:"target_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// AuditRecord is the persistence shape — every field the audit_log
// table stores. The middleware builds this from the AuditEvent plus
// actor/IP/timestamp at flush time.
type AuditRecord struct {
	ID          string
	ActorUserID *string
	ActorIP     *string
	Action      string
	TargetType  *string
	TargetID    *string
	Metadata    []byte
	CreatedAt   time.Time
}

// AuditLogger is the port the audit middleware writes through. The
// infrastructure-side implementation must NOT join the caller's
// transaction — audit rows have to survive business rollbacks.
type AuditLogger interface {
	Save(ctx context.Context, rec *AuditRecord) error
}

// AuditCollector buffers events recorded during one request. Bus
// middleware creates a fresh collector per command, hands it to the
// handler via context, then drains it after the handler returns
// (even on error). The mutex covers the case where a handler kicks
// off goroutines that also Record.
type AuditCollector struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (c *AuditCollector) Record(event AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *AuditCollector) Drain() []AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

type auditCollectorKeyType struct{}

var auditCollectorKey = auditCollectorKeyType{}

func ContextWithAuditCollector(ctx context.Context, c *AuditCollector) context.Context {
	return context.WithValue(ctx, auditCollectorKey, c)
}

// AuditCollectorFromContext returns the request-scoped collector when
// the call site is inside the bus. Outside (CLI bypass, tests) it
// returns a throwaway so handlers don't have to nil-check.
func AuditCollectorFromContext(ctx context.Context) *AuditCollector {
	if c, ok := ctx.Value(auditCollectorKey).(*AuditCollector); ok {
		return c
	}
	return &AuditCollector{}
}

type actorIPKeyType struct{}

var actorIPKey = actorIPKeyType{}

// ContextWithActorIP injects the caller's IP — populated by HTTP
// middleware right after IP extraction, consumed by the audit
// middleware when stamping AuditRecord.ActorIP.
func ContextWithActorIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, actorIPKey, ip)
}

func ActorIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(actorIPKey).(string)
	return ip
}
