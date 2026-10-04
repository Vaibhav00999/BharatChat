# Accept only backend settings, never shell statements or Compose interpolation.
def required: [
  "JWT_ACCESS_SECRET", "POSTGRES_HOST", "POSTGRES_USER", "POSTGRES_PASSWORD",
  "POSTGRES_DB", "REDIS_HOST", "REDIS_PASSWORD", "REDIS_TLS_SERVER_NAME",
  "OTP_MSG91_AUTH_KEY", "OTP_MSG91_TEMPLATE_ID", "OTP_ALLOWED_PHONES"
];
def optional: [
  "JWT_ACCESS_TTL_MINUTES", "JWT_REFRESH_TTL_DAYS", "POSTGRES_PORT", "REDIS_PORT",
  "OTP_TTL_MINUTES", "OTP_MAX_ATTEMPTS", "OTP_REQUEST_RATE_LIMIT",
  "OTP_REQUEST_RATE_WINDOW_MINUTES"
];
if type != "object" then error("Secret must be a JSON object") else . end
| if all(to_entries[];
    (.key as $key | (required + optional | index($key)) != null)
    and (.value | type == "string")
    and (.value | length > 0)
    and (.value | test("[\u0000-\u001f\u007f]") | not))
  then . else error("Unsupported key or invalid string in secret") end
| . as $secret
| if all(required[]; $secret[.] != null) then .
  else error("Required backend settings are missing") end
| if (.JWT_ACCESS_SECRET | length >= 32)
    and (.OTP_MSG91_AUTH_KEY | length >= 16)
    and (.OTP_MSG91_TEMPLATE_ID | test("^[A-Za-z0-9]{10,64}$"))
    and (.POSTGRES_HOST | endswith(".rds.amazonaws.com"))
    and (.REDIS_HOST | endswith(".cache.amazonaws.com"))
    and (.REDIS_TLS_SERVER_NAME == .REDIS_HOST)
  then . else error("Invalid AWS endpoint, TLS identity, or credential format") end
| (.OTP_ALLOWED_PHONES | split(",")) as $phones
| if ($phones | length <= 20)
    and all($phones[]; test("^\\+[1-9][0-9]{7,14}$"))
    and (($phones | unique | length) == ($phones | length))
  then . else error("Invalid pilot phone allowlist") end
| to_entries | sort_by(.key) | map("\(.key)=\(.value)") | join("\n")
