package util

import "strings"

const (
	defaultMinNotionalUSD  = 5.0
	safetyBufferMultiplier = 1.15
)

var minNotionalOverrides = map[string]float64{
	"BTCUSDT": 110.0,
	"ETHUSDT": 60.0,
}

func normalizeSymbolForRules(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

// GetMinNotionalUSD returns the minimum notional value Binance imposes for a symbol.
func GetMinNotionalUSD(symbol string) float64 {
	norm := normalizeSymbolForRules(symbol)
	if v, ok := minNotionalOverrides[norm]; ok {
		return v
	}
	return defaultMinNotionalUSD
}

// GetSafeMinPositionUSD returns the minimum USD position size we require from AI decisions.
func GetSafeMinPositionUSD(symbol string) float64 {
	norm := normalizeSymbolForRules(symbol)
	minNotional := GetMinNotionalUSD(norm)
	if norm == "BTCUSDT" || norm == "ETHUSDT" {
		return minNotional
	}
	return minNotional * safetyBufferMultiplier
}
