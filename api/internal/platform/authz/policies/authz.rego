# KlubHub Promoter authorisation policy (plan §13.2).
#
# Default deny. A request is allowed only when the action is known, one of the
# principal's roles grants it, and no deny rule fires. Every refusal carries a
# machine-readable reason for the audit log. There is no fallback for unknown
# actions or routes.
package klubhub.promoter.authz

import rego.v1

# Every action a route may declare. Unknown actions are denied.
known_actions := {
	"org.read", "org.update", "org.delete",
	"account.self", "member.read", "member.manage", "security.manage",
	"event.read", "event.write",
	"guestlist.read", "guestlist.write",
	"door.read", "door.checkin", "door.device.manage",
	"audience.read", "audience.write", "audience.export",
	"campaign.read", "campaign.write", "campaign.send",
	"booking.read", "booking.write", "artist_fee.read",
	"finance.read", "finance.write", "finance.approve",
}

# Role grants. `owner` is the only role with security and destructive org actions.
role_grants := {
	"owner": known_actions,
	"admin": known_actions - {"org.delete", "security.manage"},
	"booker": {
		"account.self", "org.read", "member.read", "event.read", "event.write",
		"guestlist.read", "guestlist.write", "door.read",
		"booking.read", "booking.write", "artist_fee.read",
		"door.device.manage",
	},
	"finance": {
		"account.self", "org.read", "member.read", "event.read", "booking.read",
		"artist_fee.read", "finance.read", "finance.write", "finance.approve",
	},
	"marketing": {
		"account.self", "org.read", "event.read", "guestlist.read",
		"audience.read", "audience.write", "campaign.read",
		"campaign.write", "campaign.send",
	},
	# Door staff: event-scoped, no export, no edits outside check-in.
	"door": {"door.read", "door.checkin"},
}

# Actions that need a second factor in the current session.
mfa_actions := {
	"org.delete", "member.manage", "security.manage", "audience.export",
	"finance.read", "finance.write", "finance.approve", "artist_fee.read",
}

# Actions that need a recent authentication (step-up), in seconds.
step_up_max_age := 900

step_up_actions := {"org.delete", "member.manage", "security.manage", "audience.export", "finance.approve"}

mfa_methods := {"mfa", "otp", "totp", "hwk", "swk", "webauthn"}

granted if {
	some role in input.principal.roles
	input.action in role_grants[role]
}

deny contains "unknown_action" if not input.action in known_actions

deny contains "no_role_grant" if {
	input.action in known_actions
	not granted
}

deny contains "tenant_mismatch" if {
	input.resource.org_id
	input.resource.org_id != input.principal.org_id
}

deny contains "no_tenant" if not input.principal.org_id

deny contains "mfa_required" if {
	input.action in mfa_actions
	count({m | some m in input.principal.amr; m in mfa_methods}) == 0
}

deny contains "reauthentication_required" if {
	input.action in step_up_actions
	input.context.now - input.principal.auth_time > step_up_max_age
}

# A principal whose only role is `door` is bound to one event.
door_only if {
	count(input.principal.roles) == 1
	"door" in input.principal.roles
}

deny contains "outside_event_scope" if {
	door_only
	input.resource.event_id != input.principal.event_scope
}

deny contains "outside_event_scope" if {
	door_only
	not input.resource.event_id
}

# Approving a payable above the organisation's threshold needs the owner.
deny contains "owner_approval_required" if {
	input.action == "finance.approve"
	input.resource.amount_minor > input.context.approval_threshold_minor
	not "owner" in input.principal.roles
}

default allow := false

allow if count(deny) == 0

# Most fundamental reason first, so "your role cannot do this" is never
# reported as "add a second factor".
reason_priority := [
	"unknown_action", "no_tenant", "tenant_mismatch", "no_role_grant",
	"outside_event_scope", "owner_approval_required",
	"mfa_required", "reauthentication_required",
]

default deny_reason := ""

deny_reason := [r | some r in reason_priority; r in deny][0] if count(deny) > 0

decision := {"allow": allow, "deny_reason": deny_reason}
