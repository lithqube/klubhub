// Package authz is the Promoter policy decision point: OPA compiled into the
// binary from embedded Rego (plan §13.2). There is no sidecar, so there is
// no "policy engine unreachable" failure mode; any evaluation error denies.
package authz

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"
)

//go:embed policies/*.rego
var policyFS embed.FS

// Principal is the verified caller. Both identity providers (local for
// self-host, Zitadel for SaaS) produce this shape.
type Principal struct {
	Sub      string    `json:"sub"`
	OrgID    string    `json:"org_id"`
	Roles    []string  `json:"roles"`
	AMR      []string  `json:"amr"`
	AuthTime time.Time `json:"-"`
	// EventScope binds door-only sessions to one event.
	EventScope string `json:"event_scope,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
}

// Resource describes what the action touches.
type Resource struct {
	Type        string `json:"type"`
	ID          string `json:"id,omitempty"`
	OrgID       string `json:"org_id,omitempty"`
	EventID     string `json:"event_id,omitempty"`
	OwnerSub    string `json:"owner_sub,omitempty"`
	AmountMinor int64  `json:"amount_minor,omitempty"`
}

// Request is one authorisation question.
type Request struct {
	Principal Principal
	Action    string
	Resource  Resource
	Route     string
	Method    string
	// ApprovalThresholdMinor is the org's payable amount above which
	// finance.approve needs the owner.
	ApprovalThresholdMinor int64
	Now                    time.Time
}

// Decision is the policy result. Reason is set whenever Allow is false.
type Decision struct {
	Allow  bool
	Reason string
}

// Reason used when the engine itself fails.
const ReasonPolicyError = "policy_error"

// Engine evaluates the embedded policy.
type Engine struct {
	query rego.PreparedEvalQuery
}

// New compiles the embedded policies. Test files are excluded.
func New(ctx context.Context) (*Engine, error) {
	opts := []func(*rego.Rego){rego.Query("data.klubhub.promoter.authz.decision")}
	err := fs.WalkDir(policyFS, "policies", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "_test.rego") {
			return err
		}
		src, err := policyFS.ReadFile(path)
		if err != nil {
			return err
		}
		opts = append(opts, rego.Module(path, string(src)))
		return nil
	})
	if err != nil {
		return nil, err
	}
	q, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("authz: compile policy: %w", err)
	}
	return &Engine{query: q}, nil
}

// Authorize evaluates req. It never returns an error: any failure is a deny
// with ReasonPolicyError.
func (e *Engine) Authorize(ctx context.Context, req Request) Decision {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	roles := req.Principal.Roles
	if roles == nil {
		roles = []string{}
	}
	amr := req.Principal.AMR
	if amr == nil {
		amr = []string{}
	}
	principal := map[string]any{
		"sub": req.Principal.Sub, "roles": roles, "amr": amr,
		"auth_time": req.Principal.AuthTime.Unix(),
	}
	if req.Principal.OrgID != "" {
		principal["org_id"] = req.Principal.OrgID
	}
	if req.Principal.EventScope != "" {
		principal["event_scope"] = req.Principal.EventScope
	}
	resource := map[string]any{"type": req.Resource.Type}
	for k, v := range map[string]string{
		"id": req.Resource.ID, "org_id": req.Resource.OrgID,
		"event_id": req.Resource.EventID, "owner_sub": req.Resource.OwnerSub,
	} {
		if v != "" {
			resource[k] = v
		}
	}
	if req.Resource.AmountMinor != 0 {
		resource["amount_minor"] = req.Resource.AmountMinor
	}
	input := map[string]any{
		"principal": principal,
		"action":    req.Action,
		"resource":  resource,
		"context": map[string]any{
			"now": now.Unix(), "route": req.Route, "method": req.Method,
			"approval_threshold_minor": req.ApprovalThresholdMinor,
		},
	}
	rs, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil || len(rs) != 1 || len(rs[0].Expressions) != 1 {
		return Decision{Reason: ReasonPolicyError}
	}
	out, ok := rs[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return Decision{Reason: ReasonPolicyError}
	}
	allow, _ := out["allow"].(bool)
	reason, _ := out["deny_reason"].(string)
	if allow {
		return Decision{Allow: true}
	}
	if reason == "" {
		reason = ReasonPolicyError
	}
	return Decision{Reason: reason}
}
