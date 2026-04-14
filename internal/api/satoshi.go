package api

// Satoshi represents a Bitcoin amount in satoshis (the smallest Bitcoin unit).
// 1 BTC = 100,000,000 satoshis.
type Satoshi int64

// SatoshiFromBTC converts a BTC float64 to Satoshi.
func SatoshiFromBTC(btc float64) Satoshi {
	return Satoshi(btc * 1e8)
}

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
