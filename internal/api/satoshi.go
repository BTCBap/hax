package api

import (
	"errors"
	"strconv"
	"strings"
)

// Satoshi represents a Bitcoin amount in satoshis (the smallest Bitcoin unit).
// 1 BTC = 100,000,000 satoshis.
type Satoshi int64

// Int64 returns the raw int64 value.
func (sat Satoshi) Int64() int64 { return int64(sat) }

// ToBTC converts satoshis to BTC as a float64.
func (sat Satoshi) ToBTC() float64 { return float64(sat) / 1e8 }

// Abs returns the absolute value.
func (sat Satoshi) Abs() Satoshi {
	if sat < 0 {
		return -sat
	}
	return sat
}

// MaxMoney is the total bitcoin supply cap in satoshis (21,000,000 BTC).
const MaxMoney Satoshi = 21_000_000 * 100_000_000

// ParseBTC parses a decimal BTC string such as "0.29" into satoshis exactly,
// without float rounding (0.29 * 1e8 in float64 truncates to 28999999).
// It rejects signs, exponents, NaN/Inf, more than 8 decimal places, zero and
// amounts above MaxMoney. Surrounding whitespace is ignored.
func ParseBTC(s string) (Satoshi, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "." {
		return 0, errors.New("empty amount")
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 8 {
		return 0, errors.New("too many decimal places (max 8)")
	}
	for _, c := range whole + frac {
		if c < '0' || c > '9' {
			return 0, errors.New("amount must be a plain decimal number")
		}
	}
	if len(strings.TrimLeft(whole, "0")) > 8 {
		return 0, errors.New("amount exceeds 21,000,000 BTC")
	}
	frac += strings.Repeat("0", 8-len(frac))
	var w, f int64
	if whole != "" {
		var err error
		if w, err = strconv.ParseInt(whole, 10, 64); err != nil {
			return 0, err
		}
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	sats := Satoshi(w*100_000_000 + f)
	if sats <= 0 {
		return 0, errors.New("amount must be greater than zero")
	}
	if sats > MaxMoney {
		return 0, errors.New("amount exceeds 21,000,000 BTC")
	}
	return sats, nil
}
