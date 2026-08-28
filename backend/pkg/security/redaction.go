package security

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	mediaCredentialPattern = regexp.MustCompile(`\b(?:xmk|xvt|xss)_[A-Za-z0-9._-]{12,}\b`)
	sensitiveValuePattern  = regexp.MustCompile(`(?i)\b(access_token|csrf(?:_token)?|authorization|cookie|password|secret)=([^&\s]+)`)
)

// RedactProviderURL retains only the scheme and host needed to identify a
// configured source. Provider usernames, paths, query credentials, and
// fragments must never appear in API responses or support diagnostics.
func RedactProviderURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "Configured source"
	}
	return parsed.Scheme + "://" + parsed.Host + "/…"
}

// RedactSensitiveText is a final safety net for diagnostics. Structured code
// should avoid logging these values in the first place, but upstream errors
// and player telemetry are not always under Xivi's control.
func RedactSensitiveText(value string) string {
	value = mediaCredentialPattern.ReplaceAllString(value, "[credential redacted]")
	return sensitiveValuePattern.ReplaceAllString(value, "$1=[redacted]")
}
