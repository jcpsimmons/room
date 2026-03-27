package prompt

import (
	"regexp"
	"strings"
)

var (
	privateKeyBlockRe = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
	npmAuthTokenRe    = regexp.MustCompile(`(?m)^(\s*//[^=\n]+:_authToken=)([^\n]*)$`)
	secretLineRe      = regexp.MustCompile(`(?i)([A-Za-z0-9_./:@-]*(?:SECRET|TOKEN|PASSWORD|PASSWD|PRIVATE_KEY|PRIVATEKEY|API_KEY|APIKEY|CLIENT_SECRET|ACCESS_TOKEN|REFRESH_TOKEN|ACCESS_KEY_ID|SECRET_ACCESS_KEY|SESSION_TOKEN)[A-Za-z0-9_./:@-]*\s*[:=]\s*)([^#\n]+)`)
	tokenLiteralRe    = regexp.MustCompile(`(?i)(gh[pous]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9-]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|AIza[0-9A-Za-z\-_]{35,})`)
)

func redactSensitiveText(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	text = privateKeyBlockRe.ReplaceAllString(text, "<redacted private key>")
	text = npmAuthTokenRe.ReplaceAllString(text, "$1<redacted>")
	text = secretLineRe.ReplaceAllString(text, "$1<redacted>$3")
	text = tokenLiteralRe.ReplaceAllString(text, "<redacted>")
	return text
}
