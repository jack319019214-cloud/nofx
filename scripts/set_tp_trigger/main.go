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

    symbol := flag.String("symbol", "BTCUSDT", "Futures symbol to protect")
    dbPath := flag.String("db", "config.db", "Path to config.db")
    userID := flag.String("user", "admin", "User ID who owns the exchange keys")
    pnlTarget := flag.Float64("pnl", 0.7, "Target pnl percent for take-profit trigger")
    closePct := flag.Float64("close_pct", 50, "Percentage of the current position to close when target hits")
    flag.Parse()

    if *closePct <= 0 || *closePct > 100 {
        log.Fatalf("close_pct must be within (0,100], got %.2f", *closePct)
    }

    creds, err := loadBinanceCreds(*dbPath, *userID)
    if err != nil {
        log.Fatalf("failed to load binance credentials: %v", err)
    }

    trader := trader.NewFuturesTrader(creds.apiKey, creds.secretKey)

    positions, err := trader.GetPositions()
    if err != nil {
        log.Fatalf("failed to get positions: %v", err)
        return
    }

    var position map[string]interface{}
    for _, pos := range positions {
        symbolStr, _ := pos["symbol"].(string)
        side, _ := pos["side"].(string)
        if symbolStr == *symbol && side == "long" {
            position = pos
            break
        }
    }

    if position == nil {
        log.Fatalf("no long position found for %s", *symbol)
    }

    qty, _ := position["positionAmt"].(float64)
    entryPrice, _ := position["entryPrice"].(float64)

    if qty <= 0 || entryPrice <= 0 {
        log.Fatalf("invalid position data: qty=%.6f entry=%.4f", qty, entryPrice)
    }

    quantityToClose := qty * (*closePct / 100.0)
    // avoid precision drift
    quantityToClose = math.Min(quantityToClose, qty)

    targetPrice := entryPrice * (1 + *pnlTarget/100.0)
    targetPrice = math.Round(targetPrice*10) / 10 // align to 0.1 tick for BTC

    log.Printf("Setting take profit for %s: qty %.6f (%.1f%%), entry %.2f → target %.2f (%.2f%%)",
        *symbol, quantityToClose, *closePct, entryPrice, targetPrice, *pnlTarget)

    if err := trader.CancelTakeProfitOrders(*symbol); err != nil {
        log.Printf("⚠️  failed to cancel existing TP orders: %v", err)
    }

    if err := trader.SetTakeProfit(*symbol, "LONG", quantityToClose, targetPrice); err != nil {
        log.Fatalf("failed to set take profit: %v", err)
    }

    log.Printf("✅ take-profit submitted: %s qty %.6f at %.2f", *symbol, quantityToClose, targetPrice)
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
