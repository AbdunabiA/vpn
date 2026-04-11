# ADR-006: Telegram-based account recovery

**Status:** Proposed (design only, not yet implemented)
**Date:** 2026-04-11
**Author:** Нео + Claude
**Supersedes:** none

## Context

The VPN app currently uses guest-only authentication keyed on a device
fingerprint (`ANDROID_ID` / `IDFV` + a client-generated secret hashed
server-side). This works for the day-to-day case but fails predictably
whenever a device identifier rotates:

- **iOS**: uninstalling the last app from a vendor resets the IDFV,
  along with app private storage — so a reinstall is indistinguishable
  from a fresh install. Users lose their premium subscription.
- **Android**: a factory reset or ROM flash issues a new ANDROID_ID,
  same result. New phone, new identifier, lost account.
- **Cross-platform**: an Android user who switches to iPhone (or vice
  versa) cannot reach their old subscription at all — the two
  identifiers are unrelated by design.

Today's recovery workflow is "the user messages `@flawlssr` on
Telegram, describes their situation, and the admin manually reassigns
their device in the panel." This works for a handful of support
tickets but isn't scalable, isn't auditable, and puts a soft requirement
of "admin trusts the user's story" at the heart of the recovery flow.

We need a recovery channel that survives both device rotations and
platform switches, with zero external infrastructure beyond what we
already run.

## Decision

Use the user's Telegram numeric user ID (`from.id` in the Bot API) as
a stable recovery identifier. The user opts in once, after their first
premium activation. On any future device, they can restore their
subscription by interacting with a dedicated VPN recovery bot.

Telegram user IDs are the right fit because:

- They don't change when the user swaps phones, changes their phone
  number, changes their username, or reinstalls the Telegram app.
- The user already contacts `@flawlssr` via Telegram to pay for
  premium, so the trust context and audit trail already exist in one
  place.
- We don't need email/SMTP, we don't need OAuth, we don't need a
  password, we don't need to store any new PII beyond a 64-bit
  integer.

The rest of this document specifies the flow.

## Architecture

### Storage — migration 015

Add two nullable columns to `users`:

```sql
ALTER TABLE users
    ADD COLUMN telegram_user_id BIGINT UNIQUE,
    ADD COLUMN telegram_linked_at TIMESTAMPTZ;

CREATE INDEX idx_users_telegram_user_id ON users(telegram_user_id)
    WHERE telegram_user_id IS NOT NULL;
```

- `telegram_user_id`: the stable numeric ID from Telegram
  (`from.id`). UNIQUE so two VPN accounts can't share a Telegram
  recovery binding — if that ever becomes a real use case, drop the
  constraint and handle the ambiguity in the recovery prompt.
- `telegram_linked_at`: when the binding was established, for audit
  and debugging.

Both columns are nullable and independent of any existing auth
columns. This is additive — the guest-only flow continues to work
unchanged for users who never opt in.

### New bot — `@vpn_mydayai_recovery_bot` (working name)

A dedicated Telegram bot, not `@flawlssr` (which is a personal
account, not a bot, and not an API client). The bot:

- Runs inside the existing Go API backend as a long-polling goroutine
  started at app init. No webhook is used because Telegram webhooks
  only accept ports 443/80/88/8443, and 443 is owned by the xray
  tunnel on our host.
- Reuses the existing DB connection pool, zap logger, and
  deployment pipeline. No new service, no new container, no new
  deploy target.
- Needs a new env var `TELEGRAM_RECOVERY_BOT_TOKEN` added to the
  production `.env` and `docker-compose.prod.yml`.
- Starts `getUpdates` with a 30-second long-poll timeout. Backs off
  on errors.

Commands handled:

| Command payload | Meaning |
|---|---|
| `/start link_<jwt>` | User is linking their Telegram account to the VPN user_id encoded in the JWT. |
| `/start restore_<jwt>` | User is restoring a previous VPN account onto the device referenced by the JWT. |
| `/start` (no payload) | Help text with brief instructions to open the VPN app and tap Link or Restore. |
| Any other text | Same help text. |

### Short-lived signed tokens

The deep links carry a short-lived (60 s) JWT signed with the same
`JWTSecret` the auth system already uses. Claim set:

```json
{
  "sub": "<vpn user_id>",
  "purpose": "tg_link" | "tg_restore",
  "iat": <unix>,
  "exp": <unix+60>
}
```

Purpose claims prevent replay — a link token can't be submitted as a
restore token and vice versa. Sixty seconds is enough for a user to
tap through Telegram and much less than the window needed to
brute-force a UUID.

### Link flow

1. User is logged into the VPN app with premium active.
2. In the Account screen, a new card "Привязать Telegram для
   восстановления аккаунта" with a single button.
3. Tapping it calls `POST /auth/telegram/link-intent` on the backend.
4. Backend generates a `tg_link` JWT for the caller's user_id and
   returns `{url: "https://t.me/vpn_mydayai_recovery_bot?start=link_<jwt>"}`.
5. App opens the URL via `Linking.openURL`. Telegram opens, shows the
   bot, the user taps "Start" (localised).
6. Bot receives `/start link_<jwt>`, validates signature + purpose +
   expiry, extracts user_id.
7. Bot calls internal handler `POST /auth/telegram/confirm-link` with
   `{user_id, telegram_user_id: <from.id>}` (this handler is
   localhost-only — not exposed via nginx).
8. Backend updates `users.telegram_user_id` + `telegram_linked_at`,
   returns OK.
9. Bot sends a Russian confirmation message: "✅ Ваш аккаунт VPN
   привязан к этому Telegram. Теперь вы можете восстановить доступ
   на любом новом устройстве."
10. The app polls `/auth/telegram/status` (or refetches the user
    account record) and shows "Привязано ✓".

### Restore flow

1. User installs VPN on a new device. `/auth/guest` mints a fresh
   guest user_id as usual.
2. On the login/onboarding screen, a new button "Восстановить
   подписку через Telegram".
3. Tapping it calls `POST /auth/telegram/restore-intent` with the
   new guest's JWT in the Authorization header.
4. Backend generates a `tg_restore` JWT for the CURRENT guest
   user_id (the one that was just minted) and returns the deep link
   URL.
5. App opens Telegram, user taps Start.
6. Bot receives `/start restore_<jwt>`, validates the token, extracts
   the new guest user_id.
7. Bot looks up the sender's `from.id`, finds an existing user row
   where `telegram_user_id = from.id`.
   - If no match: reply "❌ Этот Telegram-аккаунт не привязан ни к
     одной VPN-учётной записи. Сначала привяжите его на старом
     устройстве, если оно ещё доступно, либо напишите
     @flawlssr."
   - If the old and new user_ids are the same: reply "Этот аккаунт
     уже привязан к этому Telegram."
8. Bot sends an inline keyboard confirmation: "Восстановить подписку
   на это устройство?" with Yes/No buttons.
9. On Yes: bot calls internal `POST /auth/telegram/confirm-restore`
   with `{old_user_id, new_user_id, telegram_user_id}`.
10. Backend performs the merge transactionally:
    - Verify `old_user.telegram_user_id == telegram_user_id` (defence
      against a token that forged old_user_id).
    - Re-point every row in `devices` that belongs to `new_user_id`
      to `old_user_id`. There is typically exactly one: the device
      the user is holding right now.
    - Delete `new_user_id` (CASCADE cleans up sessions and any stray
      rows).
    - Write an audit entry with action=`tg_restore`, target_id=
      old_user_id, details={old: ..., new: ..., telegram_id: ...}.
11. Bot replies "✅ Подписка восстановлена. Откройте VPN, потребуется
    выйти и войти ещё раз."
12. The mobile app, on returning to the foreground, gets a 401 on
    the next request (because `new_user_id` no longer exists), the
    axios interceptor tries to refresh with the stale refresh token
    (which 401s — its session row belonged to the deleted
    new_user_id), and falls back to `/auth/guest` with the same
    device fingerprint. That call now finds the device row rebound
    to `old_user_id` and returns the old user's tokens. Subscription
    is restored without the user having to explicitly log out.

### Backend endpoints (new)

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/api/v1/auth/telegram/link-intent` | guest/admin JWT | Returns a link URL + 60 s JWT for the current user. |
| POST | `/api/v1/auth/telegram/restore-intent` | guest JWT | Returns a restore URL + 60 s JWT for the current (new-device) guest user. |
| GET | `/api/v1/account/telegram-status` | guest JWT | Returns `{linked: bool, linked_at: string|null}` for the current user. Used by the "Привязано ✓" UI. |

Internal (localhost only, not in nginx):

| Method | Path | Purpose |
|---|---|---|
| POST | `/internal/telegram/confirm-link` | Bot → backend: binds a telegram_user_id to a vpn user_id after token validation. |
| POST | `/internal/telegram/confirm-restore` | Bot → backend: merges new_user_id into the old user row that owns the telegram_user_id, transactionally. |

The internal endpoints are reachable only from `127.0.0.1` via a
second Fiber listener or a middleware IP check. Nginx does not
expose them.

### Version gate

All the new `/auth/telegram/*` routes need to bypass the mobile
version gate since the mobile client signs them. The simplest path:
add an exact skip for each, or a prefix skip for `/api/v1/auth/`.
Current skip list already has `/api/v1/auth/refresh` and
`/api/v1/auth/admin-login`, so this is a couple of entries in
`cmd/main.go`.

### Audit trail

Both link and restore actions land in the existing `audit_log`
table via the middleware that already wraps `/admin/*`. The bot
goroutine calls the internal endpoints, which sit INSIDE the Go
process but should still insert audit rows — either by wiring the
audit middleware to the internal listener too, or by having the
internal handler write audit rows directly (simpler, recommended).

The `action` field uses two new names: `tg_link` and `tg_restore`.
The `target_id` points to the affected user. `admin_id` is a
sentinel UUID for "system" (or the bot's synthetic admin row — we
create one by hand during rollout).

### Mobile app changes

- Account screen: new "Привязать Telegram" card, visible only when
  the user has premium (free users don't need recovery — they can
  just re-sign in as a fresh guest).
- Login/onboarding screen: new "Восстановить через Telegram" button.
- New axios wrappers for `/auth/telegram/link-intent` and
  `/auth/telegram/restore-intent`.
- `Linking.openURL` to open the Telegram deep link.
- After linking: poll `/account/telegram-status` once to show the
  "Привязано ✓" state.

### Admin panel changes

- Settings → Account card: show `telegram_user_id` if present (just
  the number; the UI doesn't resolve it to a @username).
- Users table: new optional column "Telegram" showing a checkmark
  for linked users.
- Activity page: new action types `tg_link` and `tg_restore` get
  their own colour (green-ish — positive recovery events).
- Stats card or analytics widget: "X% of premium users have linked
  Telegram".

## Consequences

### Good

- Cross-platform recovery works without email, SMTP, OAuth, or any
  external dependency beyond the Telegram Bot API (which is free).
- Users don't memorise passwords or backup codes.
- The trust anchor is already where our payment flow lives, so
  users don't have to learn a new support channel.
- The whole thing is implementable in ~2 sessions: backend + bot in
  one, mobile + admin UI in the other.
- The audit trail is richer than today — manual admin merges are
  replaced by typed actions with a target user.

### Bad / to watch

- The Telegram Bot API has no way to authenticate a specific
  Telegram account beyond "the user is the one sending us this
  message". So if an attacker steals a user's phone AND their VPN
  app, they can initiate a restore from the phone's Telegram and
  recover the account onto a controlled device. This is broadly
  equivalent to "if you lose your phone with WhatsApp installed,
  the attacker gets your WhatsApp" — standard mobile threat model.
- The bot requires a long-poll goroutine inside the backend. If the
  backend restarts frequently (e.g. during deploys), there's a few
  seconds where the bot won't respond. Use `allowed_updates` and
  offset persistence so no messages are dropped, and consider a
  small retry note in the bot's UX.
- Telegram's Bot API has rate limits (30 messages per second across
  all chats, 20 per second per chat). Fine for our scale.
- Users who block the bot in Telegram can't receive replies. Bot's
  send should be best-effort — if it fails with 403, log it,
  complete the restore anyway, and show the success state in the
  app when it next polls.
- The `telegram_user_id UNIQUE` constraint means a user can only
  link ONE VPN account to their Telegram. If someone shares their
  Telegram with a family member, only one of them can have a
  linked recovery. Acceptable for now; revisit if it ever becomes
  a real complaint.

### Alternatives considered

- **Email + OTP**: works but needs SMTP infrastructure (Resend/
  Postmark free tier), adds PII storage, and the typical user
  doesn't want to enter their email for a VPN service.
- **Sign in with Apple / Google OAuth**: cleanest UX but requires
  Apple Developer account setup, Google Cloud OAuth client, and
  real per-platform SDK integration. 10+ hours of work.
- **Backup recovery codes**: 3 hours to build, ~30% real-world
  reliability. Users lose the codes.
- **Do nothing, keep manual admin merges**: scales to the first
  few users, breaks under support load, not auditable.

Telegram wins on effort × reliability × matches-existing-workflow.

## Rollout plan

1. **Phase TG-1 (backend + bot)**
   - Migration 015 (two nullable columns)
   - Bot goroutine, `TELEGRAM_RECOVERY_BOT_TOKEN` env var
   - New routes: `/auth/telegram/{link-intent, restore-intent}`,
     `/account/telegram-status`, internal `/internal/telegram/*`
   - Version gate skip entries
   - Unit tests for token generation, claim validation, merge
     logic (transactional)
   - Deploy + smoke test via a temporary test chat

2. **Phase TG-2 (mobile)**
   - Account screen "Привязать Telegram" card
   - Login screen "Восстановить через Telegram" button
   - Open Telegram via `Linking.openURL` with the deep-link URL
     returned by the intent endpoint
   - Poll `/account/telegram-status` after link
   - Ship in a mobile app version bump (`2.1.0`), update
     `MIN_APP_VERSION`

3. **Phase TG-3 (admin panel)**
   - Telegram column in users table
   - Telegram status in Settings account card
   - `tg_link` / `tg_restore` colours in Activity page
   - Analytics card: % of premium users with Telegram linked

4. **Comms**
   - Update PaymentScreen's Telegram message template to hint at
     "После оплаты привяжите ваш Telegram через приложение, чтобы
     не потерять подписку при смене устройства."

## Open questions (for user review)

1. **Bot name** — prefer `@vpn_mydayai_recovery_bot`, or something
   shorter? Telegram bot usernames are first-come first-served;
   register early.
2. **UNIQUE constraint on telegram_user_id** — acceptable for MVP
   or do we want to support multi-account-per-Telegram from day one?
3. **Do we want the admin to be notified in Telegram when a
   restore happens?** Cheap to add — bot sends `@flawlssr` a DM
   with the details. Good audit-in-realtime.
4. **What should the bot say when a user sends unrelated messages
   (chat, stickers, voice)?** Default: the help text. Alternative:
   silent ignore. Default is friendlier.
5. **Should we let a user re-link to a different Telegram?** Today
   the UI would show "already linked to TG id X" and not offer a
   new link. A destructive "Отвязать" button is trivial to add but
   potentially a footgun.
