package wallet

import (
	"strings"
	"testing"
)

const (
	t4Addr   = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	mainAddr = "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
)

func TestParseBIP21(t *testing.T) {
	cases := []struct {
		name, uri, network string
		want               PaymentURI
	}{
		{"address only", "bitcoin:" + t4Addr, NetworkTestnet, PaymentURI{Address: t4Addr}},
		{"amount label message", "bitcoin:" + t4Addr + "?amount=0.0005&label=Luke%20Jr&message=Donation", NetworkTestnet,
			PaymentURI{Address: t4Addr, Amount: 50_000, Label: "Luke Jr", Message: "Donation"}},
		{"upper-case QR form", "BITCOIN:" + strings.ToUpper(mainAddr) + "?AMOUNT=1", NetworkMainnet,
			PaymentURI{Address: mainAddr, Amount: 100_000_000}},
		{"unknown optional param ignored", "bitcoin:" + mainAddr + "?lightning=lnbc1&pj=https://x", NetworkMainnet,
			PaymentURI{Address: mainAddr}},
		{"whitespace and plus in label", "  bitcoin:" + t4Addr + "?label=coffee+shop  ", NetworkTestnet,
			PaymentURI{Address: t4Addr, Label: "coffee shop"}},
		{"control chars stripped", "bitcoin:" + t4Addr + "?label=a%1B%5B31mb", NetworkTestnet,
			PaymentURI{Address: t4Addr, Label: "a[31mb"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseBIP21(c.uri, c.network)
			if err != nil {
				t.Fatal(err)
			}
			if *got != c.want {
				t.Fatalf("got %+v, want %+v", *got, c.want)
			}
		})
	}
}

func TestParseBIP21_Rejects(t *testing.T) {
	cases := map[string]string{
		"wrong network":     "bitcoin:" + mainAddr,
		"no address":        "bitcoin:?amount=1",
		"bad amount":        "bitcoin:" + t4Addr + "?amount=1e-3",
		"negative amount":   "bitcoin:" + t4Addr + "?amount=-1",
		"comma amount":      "bitcoin:" + t4Addr + "?amount=0,5",
		"too many decimals": "bitcoin:" + t4Addr + "?amount=0.000000001",
		"zero amount":       "bitcoin:" + t4Addr + "?amount=0",
		"over max money":    "bitcoin:" + t4Addr + "?amount=21000001",
		"required param":    "bitcoin:" + t4Addr + "?req-somethingnew=1",
		"repeated amount":   "bitcoin:" + t4Addr + "?amount=1&amount=2",
		"not a URI":         t4Addr,
		"bad escape":        "bitcoin:" + t4Addr + "?label=%zz",
		"too long":          "bitcoin:" + t4Addr + "?label=" + strings.Repeat("a", 3000),
	}
	for name, uri := range cases {
		if _, err := ParseBIP21(uri, NetworkTestnet); err == nil {
			t.Errorf("%s: %q accepted", name, uri)
		}
	}
}
