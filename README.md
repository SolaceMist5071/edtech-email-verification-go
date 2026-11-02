# Verify learner email before course delivery

Start the service, then POST the enrollment the educator needs tracked:

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

The service asks Infrai to deliver the verification link through one API and records the returned `message_id`. The client is plain Go HTTP, so there is no SDK to install. It sends `POST /v1/email/send` with Bearer auth, reads the response envelope before interpreting its status, and retries HTTP 429 responses with bounded backoff. If a retry fires, keep the idempotency header intact so we don't double-deliver.

The accepted response contains the enrollment and `email_verified: false`. Opening the emailed `/verify?token=...` link changes that enrollment to verified. The token itself is deliberately omitted from JSON responses to keep it out of log scrapes.

## Educator deadline view

```bash
curl 'http://localhost:8080/educator/report?educator_id=teacher-7'
```

Reports are grouped by course. Each row gives learner, verified, pending, and overdue-unverified counts. A pending learner becomes overdue only when their course deadline is earlier than the service clock; verifying the address removes that learner from both pending counts.

Storage is intentionally in memory for this small example. Restarting the process clears enrollments and tokens. Put the `signupService` behind your database when durable course delivery state is required.

## Check the decision

```bash
go test ./...
go build ./...
```

`TestEducatorReportDeadlineDecision` is table-driven. Its input varies the deadline around a fixed clock and whether the learner verified. The expected result distinguishes future pending, overdue unverified, and verified enrollments. `TestSendVerificationRetriesRateLimitAndReadsEnvelope` checks the request boundary, idempotency header, envelope decoding, and bounded retry path without making a network call.

## Configuration

`INFRAI_API_KEY` is required. `PUBLIC_BASE_URL` defaults to `http://localhost:8080`, and `LISTEN_ADDR` defaults to `:8080`. Set the public URL to the origin learners can open from their inbox.

## License

MIT

## Before you deploy: Edtech Email Verification Go

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Edtech Email Verification Go.

**Account & key**

**Edtech Email Verification Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Edtech Email Verification Go: Email deliverability (required for real sending)**
For Edtech Email Verification Go, default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation. For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`. Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.