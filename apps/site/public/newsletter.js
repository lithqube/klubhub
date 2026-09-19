/**
 * Newsletter signup — Plunk public track endpoint.
 *
 * Endpoint:  POST https://next-api.useplunk.com/v1/track
 * Auth:      Bearer pk_<your-public-key>  (read from <meta name="plunk-public-key">)
 * Body:      { email, event: "klubhub.subscribed" }
 *
 * The pk_* key is safe to ship in static HTML; Plunk issues it exactly
 * for browser-side event tracking. It cannot send emails or read
 * contacts — only upsert the contact and fire workflows.
 *
 * Configure the public key per environment at build time:
 *   PLUNK_PUBLIC_KEY (production) | PLUNK_PUBLIC_KEY_STAGING | PLUNK_PUBLIC_KEY_PREVIEW
 * The static-site build script (apps/site/build.mjs) injects the value
 * into <meta name="plunk-public-key"> in the rendered HTML and fails
 * closed if the key is missing, still the placeholder, or wrong-prefixed.
 *
 * Three independent Plunk projects (staging, production, preview) keep a
 * leaked staging key from touching the production newsletter list.
 */
(function () {
  'use strict';

  var TRACK_ENDPOINT = 'https://next-api.useplunk.com/v1/track';

  /**
   * Read the Plunk public key from the page meta tag. Returning null is
   * intentional — the form stays visible, but submits surface a clear
   * configuration error so the operator notices the missing meta tag.
   *
   * @returns {string|null}
   */
  function getPublicKey() {
    var meta = document.querySelector('meta[name="plunk-public-key"]');
    if (!meta) return null;
    var value = (meta.getAttribute('content') || '').trim();
    if (!value || value.indexOf('pk_') !== 0) return null;
    return value;
  }

  /**
   * Validate a single email address (HTML5 input[type=email] is
   * unreliable across browsers; do it in JS too).
   *
   * @param {string} value
   * @returns {boolean}
   */
  function isValidEmail(value) {
    if (typeof value !== 'string') return false;
    var trimmed = value.trim();
    if (trimmed.length > 254) return false;
    return /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(trimmed);
  }

  /**
   * Render a status banner inside the form. Pure DOM, no libraries.
   *
   * @param {HTMLFormElement} form
   * @param {'idle'|'sending'|'success'|'error'} state
   * @param {string} [message]
   */
  function renderStatus(form, state, message) {
    var status = form.querySelector('.nl-status');
    if (!status) return;
    status.dataset.state = state;
    status.textContent = message || '';
    status.removeAttribute('hidden');
  }

  function attach(form) {
    if (form.dataset.newsletterAttached === '1') return;
    form.dataset.newsletterAttached = '1';

    var input = form.querySelector('input[name="email"]');
    var submit = form.querySelector('button[type="submit"]');
    if (!input || !submit) return;

    form.addEventListener('submit', function (event) {
      event.preventDefault();
      var email = input.value;

      if (!isValidEmail(email)) {
        input.setAttribute('aria-invalid', 'true');
        renderStatus(form, 'error', 'Enter a valid email address.');
        input.focus();
        return;
      }
      input.removeAttribute('aria-invalid');

      var publicKey = getPublicKey();
      if (!publicKey) {
        renderStatus(
          form,
          'error',
          'Newsletter is not configured. Rebuild the site with PLUNK_PUBLIC_KEY set.',
        );
        return;
      }

      submit.disabled = true;
      renderStatus(form, 'sending', 'Subscribing...');

      fetch(TRACK_ENDPOINT, {
        method: 'POST',
        headers: {
          'Authorization': 'Bearer ' + publicKey,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          email: email.trim(),
          event: 'klubhub.subscribed',
        }),
      })
        .then(function (response) {
          return response
            .json()
            .catch(function () {
              return {};
            })
            .then(function (body) {
              return { ok: response.ok, body: body };
            });
        })
        .then(function (result) {
          submit.disabled = false;
          // Plunk returns { success: true, data: { contact, event, timestamp } }
          // for the public /v1/track endpoint. A bare HTTP 200 with an
          // empty or non-envelope body is not a confirmed success; require
          // the documented shape so an upstream regression cannot silently
          // pass through a missing contact or event id.
          if (
            result.ok &&
            result.body &&
            result.body.success === true &&
            result.body.data &&
            typeof result.body.data.contact === 'string'
          ) {
            renderStatus(form, 'success', 'Subscribed. We will be in touch.');
            form.reset();
          } else {
            var msg =
              (result.body &&
                result.body.error &&
                result.body.error.message) ||
              'Could not subscribe. Try again in a moment.';
            renderStatus(form, 'error', msg);
          }
        })
        .catch(function () {
          submit.disabled = false;
          renderStatus(form, 'error', 'Network error. Try again in a moment.');
        });
    });
  }

  function init() {
    var forms = document.querySelectorAll('form[data-newsletter]');
    Array.prototype.forEach.call(forms, attach);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
