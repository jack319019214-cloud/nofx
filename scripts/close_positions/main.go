package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"

	"nofx/trader"

	_ "modernc.org/sqlite"
)

type exchangeCreds struct {
	apiKey    string
	secretKey string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	dbPath := "config.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	creds, err := loadBinanceCreds(dbPath, "admin")
	if err != nil {
		log.Fatalf("failed to load binance credentials: %v", err)
	}

	log.Println("✅ 已获取交易密钥，准备连接币安期货 ...")
	futuresTrader := trader.NewFuturesTrader(creds.apiKey, creds.secretKey)

	positions, err := futuresTrader.GetPositions()
	if err != nil {
		log.Fatalf("获取持仓失败: %v", err)
	}

	if len(positions) == 0 {
		log.Println("📭 当前无持仓，无需平仓")
		return
	}

	log.Printf("📊 检测到 %d 个持仓，开始依次平仓...", len(positions))
	for _, p := range positions {
		symbol, _ := p["symbol"].(string)
		side, _ := p["side"].(string)
		qty, _ := p["positionAmt"].(float64)

		if symbol == "" || qty == 0 {
			continue
		}

		switch side {
		case "long":
			log.Printf("➡️  平多 %s 数量 %.6f ...", symbol, qty)
			if _, err := futuresTrader.CloseLong(symbol, qty); err != nil {
				log.Fatalf("平多失败 %s: %v", symbol, err)
			}
		case "short":
			closeQty := math.Abs(qty)
			log.Printf("➡️  平空 %s 数量 %.6f ...", symbol, closeQty)
			if _, err := futuresTrader.CloseShort(symbol, closeQty); err != nil {
				log.Fatalf("平空失败 %s: %v", symbol, err)
			}
		default:
			log.Printf("⚠️  未知方向 %s (%v)，跳过", symbol, side)
		}
	}

	log.Println("🎯 所有持仓已尝试平仓完成")
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

	return &exchangeCreds{
		apiKey:    apiKey,
		secretKey: secretKey,
	}, nil
}
