# Plunk workflow contract — KlubHub

Plunk workflows are created and edited in the Plunk UI only — there is **no MCP tool for workflow mutation or introspection**. This file encodes the workflow contracts that KlubHub uses so they don't get re-derived and re-broken.

## Why custom events, not system events

The KlubHub DJ · Welcome workflow listens for a Plunk event and sends the welcome email when that event fires. Plunk's docs list `contact.subscribed` as a workflow trigger option, but in practice it **does not fire on contact upserts created via `POST /v1/track`** — confirmed live with `figu@figuds.com` (subscribed contact, zero workflow executions in the Activity tab).

The pattern Plunk's own waitlist recipe uses is namespaced custom events:

- The static site fires `event: "klubhub.subscribed"` (or any other `klubhub.<thing>` name) via `POST /v1/track`.
- The workflow trigger is set to the same name (`klubhub.subscribed`).

Custom events fire reliably. System events on `/v1/track` upserts do not.

## Trigger event name rules

- Trigger event name **is locked after the first workflow execution**. Set it correctly the first time, before the workflow fires for any contact.
- Plunk's UI does not show a "rename" option after the first execution; you'd have to clone the workflow and start over.
- Naming convention: `klubhub.<verb>.<object>` (e.g. `klubhub.subscribed`, `klubhub.welcome.queued`, `klubhub.feedback.received`).

## Event firing pattern

The static site's newsletter signup does this:

```js
// apps/site/public/newsletter.js
fetch('https://next-api.useplunk.com/v1/track', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${publicKey}`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    email: email.trim(),
    event: 'klubhub.subscribed',
  }),
})
```

The response is `{ success: true, data: { contact, event, timestamp } }` on success. The success path checks `body.success === true && body.data.contact` (per Plunk's documented envelope).

If you need to fire **two** events on the same signup (e.g. one for welcome, one for analytics), fire two `POST /v1/track` calls back-to-back. Plunk handles the dedupe of contact upsert internally.

## What MCP can do (read-mostly)

The Plunk MCP exposes 23 tools, grouped:

| Group | Read | Write |
|---|---|---|
| Templates | `plunk_list_templates`, (implicit `plunk_get_template` via ID) | `plunk_create_template` (only — no update) |
| Contacts | `plunk_list_contacts`, `plunk_get_contact` | `plunk_create_contact`, `plunk_update_contact` |
| Campaigns | `plunk_list_campaigns`, `plunk_get_campaign`, `plunk_get_campaign_stats` | `plunk_create_campaign`, `plunk_send_campaign`, `plunk_test_campaign`, `plunk_cancel_campaign` |
| Segments | `plunk_list_segments` | `plunk_create_segment` |
| Events | (none — events are read via Activity tab in UI only) | `plunk_track_event` |
| Email | (none) | `plunk_send_email`, `plunk_test_campaign` |
| Misc | — | `plunk_verify_email`, `plunk_check_domain` |

## What MCP CANNOT do (UI only)

- **No workflow mutation**: cannot create, update, or delete workflows.
- **No workflow execution introspection**: cannot list executions, get execution status, or replay events.
- **No template update**: `plunk_create_template` creates a new one; updates must happen in the UI.
- **No domain verification setup**: must be done in Plunk UI → Project → Domains.
- **No sender address configuration**: must be done in Plunk UI → Project → Senders.
- **No API key rotation**: must be done in Plunk UI → Project → API keys.

So when an operator says "fire a Plunk workflow" or "check why the welcome didn't send", the answer is almost always:

1. Open the Plunk UI → Workflows → `klubhub · welcome onboard`.
2. Confirm the workflow is **Enabled**.
3. Click **Test execution** → pick a contact → click Send.
4. If it works in Test, the workflow is fine. The original issue was likely timing (event fired before workflow was enabled).
5. If it fails in Test, read the inline error — most likely candidates: sender domain not verified, template ID missing, from-address mismatch.

## Template types and what they mean

| Type | Auto unsubscribe footer | Use case |
|---|---|---|
| **MARKETING** | Yes — Plunk auto-injects its own `{{ unsubscribeUrl }}` footer | Broadcast / newsletter sends |
| **HEADLESS** | No — you include your own footer in the body | Transactional / designed-for-app emails with explicit `{{ unsubscribeUrl }}` and `{{ manageUrl }}` |

KlubHub templates:

- `KlubHub DJ · Welcome` → **MARKETING**. Plunk's auto-injected footer sits below the custom footer. Recipient sees both — redundant but not wrong.
- `KlubHub DJ · Onboarding` → **HEADLESS**. The custom footer is the only one. Used for the broadcast announcement that goes to existing subscribers.

If you create new templates, pick the type based on whether the recipient expects Plunk's UI chrome (Marketing) or a clean self-contained email (Headless).

## Diagnostic flow

When the welcome doesn't arrive for a known signup:

1. Plunk → Activity → filter by workflow name. Any executions? Yes → check status of the SEND_EMAIL step. No → trigger event never fired.
2. Plunk → Activity → filter by event name `klubhub.subscribed`. Any events? Yes → trigger matches workflow. No → static site didn't fire, check browser console.
3. Plunk → Contacts → confirm contact exists with `subscribed: true` and recent `createdAt`.
4. If everything looks right but no execution, the workflow was likely disabled or the trigger event name drifted. Open the workflow, verify the trigger chip name matches `klubhub.subscribed` exactly.
5. If you can't resolve via MCP, this is one of the cases where the operator's Plunk UI is the only diagnostic surface. Ask the user to open the Activity tab and screenshot if needed.
