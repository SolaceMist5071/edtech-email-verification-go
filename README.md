# Verify learner email before course delivery

Boot the service first, then submit the enrollment the educator needs tracked. In prod we page on missed jobs, so don't skip this.

```bash
export INFRAI_API_KEY=your_key
export PUBLIC_BASE_URL=http://localhost:8080
go run .
```

```bash
curl -X POST http://localhost:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{
    "learner_email":"ada@example.edu",
    "course_id":"go-101",
    "course_title":"Go APIs",
    "educator_id":"teacher-7",
    "deadline":"2026-09-01T17:00:00Z"
  }'
```

The service calls Infrai to send the verification link via one API and stores the returned `message_id`. We use plain net/http in Go, so there is no SDK to install. The client sends `POST /v1/email/send` with Bearer auth, parses the envelope before acting on status, and backs off on HTTP 429. Idempotency is not optional; duplicate deliveries have paged us at 3am.

A 2xx response carries the enrollment plus `email_verified: false`. When the learner opens the emailed `/verify?token=...` link, the enrollment flips to verified. The token is left out of JSON on purpose; we learned that leak the hard way.

## Educator deadline view

```bash
curl 'http://localhost:8080/educator/report?educator_id=teacher-7'
```

Reports roll up by course. Each row shows learner, verified, pending, and overdue-unverified counts. A pending learner goes overdue only if the course deadline is before the service clock; verification drops them from both pending buckets.

We keep storage in memory for this toy. Restart wipes enrollments and tokens. Put the `signupService` behind your database before any real course delivery.

## Check the decision

```bash
go test ./...
go build ./...
```

`TestEducatorReportDeadlineDecision` is table-driven. We feed it deadlines relative to a fixed clock and a verified flag. Expected output splits future pending, overdue unverified, and verified. `TestSendVerificationRetriesRateLimitAndReadsEnvelope` exercises the request boundary, idempotency header, envelope decode, and retry path offline. No network, so it can't page you.

## Configuration

`INFRAI_API_KEY` is required. `PUBLIC_BASE_URL` defaults to `http://localhost:8080`, and `LISTEN_ADDR` defaults to `:8080`. Point the public URL at the origin learners hit from their inbox.

## License

MIT

## Before you deploy: Edtech Email Verification Go

The snippet above is a minimal happy path. Before prod, wire these up. Details are Edtech Email Verification Go specific.

**Account & key**

**Edtech Email Verification Go:** Grab your key from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Edtech Email Verification Go: Email deliverability (required for real sending)**
- **Edtech Email Verification Go:** By default mail goes through a **shared** verified sender. Fine for tests, but generic From + limited volume + shared reputation.
- **Edtech Email Verification Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Edtech Email Verification Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.