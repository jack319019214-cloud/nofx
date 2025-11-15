package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"

	"nofx/trader"

	_ "modernc.org/sqlite"
)

type exchangeCreds struct {
	apiKey    string
	secretKey string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	symbol := flag.String("symbol", "BTCUSDT", "filter symbol (optional)")
	dbPath := flag.String("db", "config.db", "path to config.db")
	userID := flag.String("user", "admin", "user owning credentials")
	flag.Parse()

	creds, err := loadBinanceCreds(*dbPath, *userID)
	if err != nil {
		log.Fatalf("failed to load creds: %v", err)
	}

	ft := trader.NewFuturesTrader(creds.apiKey, creds.secretKey)
	positions, err := ft.GetPositions()
	if err != nil {
		log.Fatalf("failed to get positions: %v", err)
	}

	if len(positions) == 0 {
		fmt.Println("no positions")
		return
	}

	for _, p := range positions {
		sym, _ := p["symbol"].(string)
		if *symbol != "" && *symbol != sym {
			continue
		}
		fmt.Printf("%s %s qty=%.6f entry=%.4f mark=%.4f leverage=%.1f\n",
			sym, p["side"], p["positionAmt"], p["entryPrice"], p["markPrice"], p["leverage"])
	}
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
