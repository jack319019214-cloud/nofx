package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"

	"nofx/trader"

	_ "modernc.org/sqlite"
)

type exchangeCreds struct {
	apiKey    string
	secretKey string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	symbol := flag.String("symbol", "BTCUSDT", "symbol to reduce")
	dbPath := flag.String("db", "config.db", "path to config.db")
	userID := flag.String("user", "admin", "user owning exchange creds")
	qty := flag.Float64("qty", 0, "absolute quantity to close (optional)")
	pct := flag.Float64("pct", 0, "percentage of current position to close (0-100)")
	flag.Parse()

	if *qty <= 0 && (*pct <= 0 || *pct > 100) {
		log.Fatalf("provide --qty > 0 or 0 < --pct <= 100")
	}

	creds, err := loadBinanceCreds(*dbPath, *userID)
	if err != nil {
		log.Fatalf("failed to load binance credentials: %v", err)
	}

	ft := trader.NewFuturesTrader(creds.apiKey, creds.secretKey)
	positions, err := ft.GetPositions()
	if err != nil {
		log.Fatalf("failed to get positions: %v", err)
	}

	var target map[string]interface{}
	for _, p := range positions {
		if sym, _ := p["symbol"].(string); sym == *symbol {
			if side, _ := p["side"].(string); side == "long" {
				target = p
				break
			}
		}
	}

	if target == nil {
		log.Fatalf("no long position found for %s", *symbol)
	}

	positionAmt, _ := target["positionAmt"].(float64)
	if positionAmt <= 0 {
		log.Fatalf("position amount is non-positive: %.6f", positionAmt)
	}

	var closeQty float64
	if *qty > 0 {
		closeQty = *qty
	} else {
		closeQty = positionAmt * (*pct / 100.0)
	}

	if closeQty <= 0 {
		log.Fatalf("calculated close quantity is zero")
	}
	if closeQty > positionAmt {
		closeQty = positionAmt
	}

	closeQty = math.Round(closeQty*1000) / 1000 // align with BTC step 0.001

	log.Printf("Closing %.3f of %s (position %.3f)", closeQty, *symbol, positionAmt)

	if _, err := ft.CloseLong(*symbol, closeQty); err != nil {
		log.Fatalf("failed to close long: %v", err)
	}

	log.Printf("✅ partial close sent for %.3f %s", closeQty, *symbol)
}

func loadBinanceCreds(dbPath, userID string) (*exchangeCreds, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	var apiKey, secretKey string
	query := `SELECT api_key, secret_key FROM exchanges WHERE id = 'binance' AND user_id = ? LIMIT 1`
	if err := db.QueryRow(query, userID).Scan(&apiKey, &secretKey); err != nil {
		return nil, fmt.Errorf("query binance exchange: %w", err)
	}

	if apiKey == "" || secretKey == "" {
		return nil, fmt.Errorf("binance exchange credentials are empty")
	}

	return &exchangeCreds{apiKey: apiKey, secretKey: secretKey}, nil
}
