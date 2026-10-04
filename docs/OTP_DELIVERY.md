# OTP Delivery

The backend supports four delivery modes without changing the public auth API:

- `console`: local development only; prints the code to backend stdout.
- `webhook`: sends the code to a deployment-owned HTTPS SMS adapter.
- `twilio`: sends the code directly through Twilio Programmable Messaging.
- `msg91`: sends our six-digit challenge through MSG91 SendOTP v5.

Non-development environments reject `console` during configuration loading.
Production delivery modes do not return or log the raw code through BharatChat's API.

During prelaunch, `OTP_ALLOWED_PHONES` is mandatory outside development. It must
contain 1-20 unique E.164 numbers separated by commas with no spaces. Keep real
tester numbers in Secrets Manager, not this document. Requests and verification
from other numbers are rejected before provider or database work. You can also
set this allowlist for a local real-SMS test. Removing a number blocks new logins,
but existing sessions require separate revocation.

## Selected Provider: MSG91

The deployment target is `bharatchat.in`, with `api.bharatchat.in` proposed for
the API. Neither hostname is provisioned by adding configuration files.
`backend/.env.production.example` contains the production configuration template.

Configure these values in the deployment secret manager, or in the ignored
`backend/.env` for a local real-SMS test:

```dotenv
OTP_DELIVERY_MODE=msg91
OTP_MSG91_AUTH_KEY=<dedicated MSG91 auth key>
OTP_MSG91_TEMPLATE_ID=<approved MSG91 OTP template ID>
OTP_TTL_MINUTES=5
```

Use the template ID from the MSG91 OTP section, not a DLT content ID. Complete
the applicable sender/entity/template approvals in the MSG91 account. The OTP
template uses `##OTP##`; extra custom variables are not supported by this adapter.
The app generates and verifies the code locally with its existing attempt limits
and single-use database challenge. MSG91 delivers that exact code; its Verify OTP
and Resend OTP APIs are not used. Requesting a replacement code goes through
BharatChat's existing request endpoint and rate limits.

The adapter uses HTTPS, a 10-second deadline, no redirects, no automatic retries,
and validates both HTTP status and the provider's success/request ID. The official
SendOTP API puts the auth key, recipient, and OTP in query parameters: outbound
URLs and provider response bodies must never be captured by HTTP tracing or logs.
The adapter deliberately redacts transport errors for this reason. MSG91 still
receives the phone number and OTP as the SMS delivery provider.

After changing environment variables, recreate the backend container. A restart
alone does not reload Docker's environment. Keep development ports loopback-only.
Provider acceptance is not proof of handset delivery: verify delivery on real
devices and inspect MSG91 delivery logs before enabling public registration.

Official references:

- [SendOTP API](https://docs.msg91.com/otp/sendotp)
- [OTP template setup](https://msg91.com/help/sendotp/where-to-find-the-sendotp-api-how-to-get-template-id)
- [DLT template mapping](https://msg91.com/help/dlt-registration-in-india/map-sms-content-template-on-msg91-api-panel)

## Twilio Setup

Create a Twilio Messaging Service with an SMS-capable sender, then create a
dedicated API key for this workload. Twilio recommends API keys for REST API
authentication; use a restricted key with only the required messaging permission
when that key type is available for the account.

Set these values through the deployment platform's secret manager:

```dotenv
OTP_DELIVERY_MODE=twilio
OTP_TWILIO_ACCOUNT_SID=AC...
OTP_TWILIO_API_KEY=SK...
OTP_TWILIO_API_SECRET=...
OTP_TWILIO_MESSAGING_SERVICE_SID=MG...
OTP_SMS_MESSAGE_TEMPLATE=Your BharatChat verification code is {{CODE}}. It expires in {{MINUTES}} minutes. Do not share it.
```

`{{CODE}}` must occur exactly once. `{{MINUTES}}` is optional and is replaced
with the configured OTP lifetime. Startup fails when credentials, SID formats,
or the code placeholder are invalid. The sender performs one request with a
10-second timeout and does not automatically retry ambiguous failures, avoiding
accidental duplicate codes.

## India Compliance

For domestic delivery to Indian numbers, complete entity and Sender ID
registration in an operator DLT portal, register the exact OTP template, and
complete Twilio's corresponding sender registration before launch. Configure
`OTP_SMS_MESSAGE_TEMPLATE` to exactly match the approved template apart from its
registered variable positions. Legal and carrier approval cannot be completed by
application code.

Twilio references:

- https://www.twilio.com/docs/messaging/api/message-resource
- https://www.twilio.com/docs/iam/api-keys
- https://www.twilio.com/en-us/guidelines/in/sms

## Release Check

Test delivery with a staging Messaging Service and real target devices on every
supported carrier before production. Monitor provider rejection and delivery
rates, alert on sustained failures, and keep the console mode disabled outside
local development.
