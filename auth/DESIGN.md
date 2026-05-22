---
package: auth
purpose: Persists federated identities and verification channels (gorm-backed) and provides HTTP middleware for login enforcement and logged-in-user propagation.
last_rebuilt: 04f5d50ab9801bba49c428990700c9aded8b119b
depends_on:
entities:
  - name: Identity
    kind: struct
    role: A globally unique "address" for a user (e.g. abc@example.com), keyed by IdentityType + IdentityKey.
    why: Deliberately decoupled from any single channel — multiple channels (Google/FB/GitHub) may verify the same identity, and PrimaryUser is optional so one identity can front several users.
  - name: NewIdentity
    kind: func
    role: Constructor that stamps CreatedAt on a new Identity.
    why: Centralizes the BaseEntity timestamp init so callers don't forget it.
  - name: Identity.HasUser
    kind: func
    role: Reports whether PrimaryUser is set (after trimming).
    why: Encodes that an identity legitimately exists without an associated user.
  - name: Identity.Key
    kind: func
    role: Returns the composite "type:key" string identifier.
    why: The pair, not either field alone, is the system-wide unique key.
  - name: Channel
    kind: struct
    role: A federated verification record (provider + loginId) holding credentials, provider profile, and expiry, pointing at an IdentityKey.
    why: Credentials/Profile are dal.JsonField stored as text so arbitrary provider payloads survive without schema changes; ExpiresAt drives re-auth.
  - name: NewChannel
    kind: func
    role: Constructor stamping CreatedAt; takes a params StrMap that it currently ignores for fields.
    why: Gotcha — the params arg is accepted but not applied here; credentials/profile are populated later in EnsureChannel, not the constructor.
  - name: Channel.HasIdentity
    kind: func
    role: Reports whether this channel is bound to an identity.
    why: A channel may exist before it is linked to an identity.
  - name: Channel.Key / Channel.HasKey
    kind: func
    role: Composite "provider:loginId" identifier and presence check.
    why: Provider + loginId is the channel's primary key, mirroring Identity's two-part key.
  - name: CallbackRequest
    kind: struct
    role: Describes an outbound callback (hostname, path, method, body, headers) for OAuth-style flows.
    why: rawBody is unexported — built/consumed within the package, not set by external callers.
  - name: CallbackRequest.FullURL
    kind: func
    role: Joins Hostname and Path into a single URL.
    why: Hand-rolled join inserts exactly one slash; note it does NOT collapse a double slash when both sides already carry one.
  - name: AuthDB
    kind: struct
    role: Gorm-backed repository for Channel and Identity entities.
    why: Wraps *gorm.DB and runs AutoMigrate on construction so the two tables exist before any call.
  - name: NewAuthDB
    kind: func
    role: Builds an AuthDB and auto-migrates Channel + Identity.
    why: Migration is a constructor side effect, so just creating the DB readies the schema.
  - name: AuthDB.SaveChannel / SaveIdentity
    kind: func
    role: Upsert helpers: Save, and if zero rows affected, Create.
    why: Gotcha — they rely on RowsAffected==0 to detect a missing row and fall back to Create, working around gorm Save not inserting composite-key rows reliably.
  - name: AuthDB.GetChannel / GetIdentity
    kind: func
    role: Lookup by composite key, returning (nil, nil) when not found.
    why: Treats ErrRecordNotFound as a non-error so callers branch on nil rather than inspecting error types.
  - name: AuthDB.EnsureChannel
    kind: func
    role: Get-or-create a channel, then overwrite its Credentials and Profile from params.
    why: Gotcha — does unchecked type assertions on params["credentials"]/["profile"] as utils.StrMap; malformed params panic.
  - name: AuthDB.EnsureIdentity
    kind: func
    role: Get-or-create an identity, returning (identity, wasNewlyCreated).
    why: Unlike EnsureChannel it ignores params entirely beyond existence — identities carry no mutable payload here.
  - name: AuthConfig
    kind: struct
    role: Configures HTTP auth behavior — session lookup, redirect URLs, user param name, and the login-redirect toggle.
    why: SessionGetter is an injected func so the package stays agnostic of the session store.
  - name: AuthConfig.EnsureReasonableDefaults
    kind: func
    role: Fills empty config fields with sensible defaults.
    why: Lets callers supply a zero-value AuthConfig; NoLoginRedirect suppresses the default GetRedirURL so unauthenticated requests get a 401 instead of a redirect.
  - name: GetLoggedInUser / SetLoggedInUser
    kind: func
    role: Read/write the logged-in user ID on a context.Context.
    why: Uses an unexported contextKey type so the key cannot collide with other packages' context values.
  - name: AuthConfig.GetLoggedInUserId
    kind: func
    role: Resolves the current user, preferring the context then falling back to the session.
    why: Context takes precedence so middleware-set users win over a stale session lookup.
  - name: AuthConfig.ExtractUserInfo
    kind: func
    role: Middleware that reads the session user and stashes it in the request context (non-blocking).
    why: Populates context for downstream handlers without enforcing login — pairs with EnsureLoginMiddleware which does enforce.
  - name: AuthConfig.EnsureLoginMiddleware / EnsureLogin
    kind: func
    role: Middleware/handler that redirects (or 401s) unauthenticated requests and sets the user in context otherwise.
    why: On redirect it round-trips the original path via CallbackURLParam (URI-encoded) so the user returns post-login.
  - name: contextKey / loggedInUserKey
    kind: const
    role: Private context-key type and the single key used for the logged-in user.
    why: Idiomatic guard against context key collisions across packages.
---

This package is the persistence + middleware half of an auth system; the actual provider/OAuth flow logic lives elsewhere (the package only models the records and the HTTP enforcement layer).

Two-part composite keys are the core data model: Identity is `(IdentityType, IdentityKey)` and Channel is `(Provider, LoginId)`, with Channel referencing Identity loosely via `IdentityKey`. The deliberate looseness — identities without users, channels without identities — supports federated login where the same email is verified by several providers.

Notable gotchas: the `Save` → `Create` fallback in `SaveChannel`/`SaveIdentity` depends on `RowsAffected==0` rather than a prior existence check; `EnsureChannel` does unchecked `utils.StrMap` type assertions on its params (panic risk); and `NewChannel`'s `params` argument is currently a no-op. The test file (`integ_test.go`) is a stub, so behavior is undocumented by tests.
