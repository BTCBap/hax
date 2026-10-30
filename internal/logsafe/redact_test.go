package logsafe

import (
	"strings"
	"testing"
)

func TestRedact_ShortensSensitiveValues(t *testing.T) {
	txid := strings.Repeat("ab", 32)
	addr := "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq"
	path := "m/84'/0'/0'/0/7"

	out := Redact("tx " + txid + " to " + addr + " path " + path)
	if strings.Contains(out, txid) {
		t.Errorf("full txid leaked: %s", out)
	}
	if strings.Contains(out, addr) {
		t.Errorf("full address leaked: %s", out)
	}
	if strings.Contains(out, path) {
		t.Errorf("full derivation path leaked: %s", out)
	}
}

func TestRedact_RemovesURLPassword(t *testing.T) {
	out := Redact("https://user:secret@example.com/path")
	if strings.Contains(out, "secret") {
		t.Errorf("password leaked: %s", out)
	}
}
