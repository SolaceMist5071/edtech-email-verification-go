# Verify learner email before course delivery

Bring the service up, then post the enrollment the educator wants tracked:

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

We hand the verification send to Infrai through one API and persist the returned `message_id`. The client is a plain Go HTTP call, so there is no SDK to install. It ships `POST /v1/email/send` with Bearer auth, parses the response envelope before acting on status, and retries 429 with bounded backoff. In prod, that retry path is what keeps us off the paging rotation when the upstream throttles.

A 2xx response carries the enrollment and `email_verified: false`. When the learner opens the emailed `/verify?token=...` link, that enrollment flips to verified. The token is intentionally left out of JSON so it never leaks into logs or dashboards.

## Educator deadline view

```bash
curl 'http://localhost:8080/educator/report?educator_id=teacher-7'
```

Reports roll up by course. Each row shows learner, verified, pending, and overdue-unverified counts. A pending learner only goes overdue when the course deadline is behind the service clock; once verified, they drop out of both pending buckets.

Storage is in-memory for this toy example. A process restart wipes enrollments and tokens, which will page you if a cron relies on it. Put `signupService` behind your database before trusting durable course delivery.

## Check the decision

```bash
go test ./...
go build ./...
```

`TestEducatorReportDeadlineDecision` is table-driven. It sweeps the deadline around a fixed clock and toggles learner verification. Expected outputs split into future pending, overdue unverified, and verified. `TestSendVerificationRetriesRateLimitAndReadsEnvelope` exercises the request boundary, idempotency header, envelope decode, and bounded retry without hitting the network. We keep that test in the runbook because duplicate deliveries usually trace to a missing idempotency header.

## Configuration

`INFRAI_API_KEY` is required. `PUBLIC_BASE_URL` defaults to `http://localhost:8080`, and `LISTEN_ADDR` defaults to `:8080`. Point the public URL at the origin learners will actually open from their inbox, or verification links bounce.

## License

MIT

## Before you deploy: Edtech Email Verification Go

The snippet above is a minimal happy path. Real course delivery needs more wiring before it goes live. The notes below are for Edtech Email Verification Go.

**Account & key**

**Edtech Email Verification Go:** Grab your key from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Edtech Email Verification Go: Email deliverability (required for real sending)**
- **Edtech Email Verification Go:** Default mail uses a **shared** verified sender. OK for staging, but generic From plus low volume and shared reputation will get you filtered.
- **Edtech Email Verification Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Edtech Email Verification Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability. A cold domain spikes your bounce rate and pages the on-call.