package klubhub.promoter.authz_test

import rego.v1

import data.klubhub.promoter.authz

org := "0190f1d2-0000-7000-8000-000000000001"

base(roles, action) := {
	"principal": {"sub": "u1", "org_id": org, "roles": roles, "amr": ["pwd"], "auth_time": 1000},
	"action": action,
	"resource": {"type": "event", "org_id": org},
	"context": {"now": 1100, "approval_threshold_minor": 100000},
}

test_booker_can_edit_events if {
	authz.decision.allow with input as base(["booker"], "event.write")
}

test_marketing_cannot_read_artist_fees if {
	d := authz.decision with input as base(["marketing"], "artist_fee.read")
	not d.allow
	d.deny_reason == "no_role_grant"
}

test_unknown_action_is_denied_even_for_owner if {
	d := authz.decision with input as base(["owner"], "something.new")
	not d.allow
	d.deny_reason == "unknown_action"
}

test_cross_tenant_resource_is_denied if {
	i := json.patch(base(["owner"], "event.read"), [{"op": "replace", "path": "/resource/org_id", "value": "other"}])
	d := authz.decision with input as i
	not d.allow
	d.deny_reason == "tenant_mismatch"
}

test_finance_needs_mfa if {
	d := authz.decision with input as base(["finance"], "finance.read")
	d.deny_reason == "mfa_required"
	i := json.patch(base(["finance"], "finance.read"), [{"op": "replace", "path": "/principal/amr", "value": ["pwd", "otp"]}])
	authz.decision.allow with input as i
}

test_security_manage_needs_recent_auth if {
	stale := json.patch(base(["owner"], "security.manage"), [
		{"op": "replace", "path": "/principal/amr", "value": ["pwd", "otp"]},
		{"op": "replace", "path": "/context/now", "value": 1000 + 901},
	])
	d := authz.decision with input as stale
	d.deny_reason == "reauthentication_required"
}

test_door_staff_are_event_scoped if {
	i := {
		"principal": {"sub": "d1", "org_id": org, "roles": ["door"], "amr": [], "auth_time": 1000, "event_scope": "e1"},
		"action": "door.checkin",
		"resource": {"type": "guest", "org_id": org, "event_id": "e1"},
		"context": {"now": 1100},
	}
	authz.decision.allow with input as i
	other := json.patch(i, [{"op": "replace", "path": "/resource/event_id", "value": "e2"}])
	d := authz.decision with input as other
	d.deny_reason == "outside_event_scope"
}

test_large_approval_needs_owner if {
	i := json.patch(base(["finance"], "finance.approve"), [
		{"op": "replace", "path": "/principal/amr", "value": ["otp"]},
		{"op": "add", "path": "/resource/amount_minor", "value": 250000},
	])
	d := authz.decision with input as i
	d.deny_reason == "owner_approval_required"
}

test_no_roles_is_denied if {
	d := authz.decision with input as base([], "org.read")
	not d.allow
	d.deny_reason == "no_role_grant"
}

test_every_staff_role_manages_its_own_account if {
	every role in ["owner", "admin", "booker", "finance", "marketing"] {
		authz.decision.allow with input as base([role], "account.self")
	}
	d := authz.decision with input as json.patch(base(["door"], "account.self"), [{"op": "add", "path": "/principal/event_scope", "value": "e1"}, {"op": "add", "path": "/resource/event_id", "value": "e1"}])
	d.deny_reason == "no_role_grant"
}
