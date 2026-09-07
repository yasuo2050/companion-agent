package observability

import (
	"regexp"
	"strings"
)

var credentialPattern = regexp.MustCompile(`(?i)((?:authorization|api[_-]?key|secret[_-]?key|password)["'\s]*[:=]["'\s]*)(?:Bearer\s+)?[^"'\s,}]+`)
var bearerPattern = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)

// masker 先替换运行时已知密钥，再处理用户输入中的凭证字段。
func masker(secrets ...string) func(string) string {
	return func(value string) string {
		for _, secret := range secrets {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[REDACTED]")
			}
		}
		value = credentialPattern.ReplaceAllString(value, "${1}[REDACTED]")
		return bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	}
}
