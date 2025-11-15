package decision

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/market"
	"nofx/mcp"
	"nofx/pool"
	"nofx/util"
	"regexp"
	"strings"
	"time"
)

// 预编译正则表达式（性能优化：避免每次调用时重新编译）
var (
	// ✅ 安全的正則：精確匹配 ```json 代碼塊
	// 使用反引號 + 拼接避免轉義問題
	reJSONFence      = regexp.MustCompile(`(?is)` + "```json\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*```")
	reJSONArray      = regexp.MustCompile(`(?is)\[\s*\{.*?\}\s*\]`)
	reArrayHead      = regexp.MustCompile(`^\[\s*\{`)
	reArrayOpenSpace = regexp.MustCompile(`^\[\s+\{`)
	reInvisibleRunes = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF]")

	// 新增：XML标签提取（支持思维链中包含任何字符）
	reReasoningTag = regexp.MustCompile(`(?s)<reasoning>(.*?)</reasoning>`)
	reDecisionTag  = regexp.MustCompile(`(?s)<decision>(.*?)</decision>`)
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // 持仓更新时间戳（毫秒）
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime     string                  `json:"current_time"`
	RuntimeMinutes  int                     `json:"runtime_minutes"`
	CallCount       int                     `json:"call_count"`
	Account         AccountInfo             `json:"account"`
	Positions       []PositionInfo          `json:"positions"`
	CandidateCoins  []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap   map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	OITopDataMap    map[string]*OITopData   `json:"-"` // OI Top数据映射
	Performance     interface{}             `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage  int                     `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                     `json:"-"` // 山寨币杠杆倍数（从配置读取）
}

// Decision AI的交易决策
type Decision struct {
	Symbol string `json:"symbol"`
	Action string `json:"action"` // "open_long", "open_short", "close_long", "close_short", "update_stop_loss", "update_take_profit", "partial_close", "hold", "wait"

	// 开仓参数
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`
	EntryPrice      float64 `json:"entry_price,omitempty"`

	// 调整参数（新增）
	NewStopLoss     float64 `json:"new_stop_loss,omitempty"`    // 用于 update_stop_loss
	NewTakeProfit   float64 `json:"new_take_profit,omitempty"`  // 用于 update_take_profit
	ClosePercentage float64 `json:"close_percentage,omitempty"` // 用于 partial_close (0-100)

	// 通用参数
	Confidence int     `json:"confidence,omitempty"` // 信心度 (0-100)
	RiskUSD    float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	Reasoning  string  `json:"reasoning"`

	// 兼容额外字段（某些模型会输出自定义schema）
	AmountType                string    `json:"amount_type,omitempty"`
	AmountValue               float64   `json:"amount_value,omitempty"`
	Percentage                float64   `json:"percentage,omitempty"`
	PercentAlias              float64   `json:"percent,omitempty"`
	Quantity                  float64   `json:"quantity,omitempty"`
	LimitPrice                float64   `json:"limit_price,omitempty"`
	OpenPrice                 float64   `json:"open_price,omitempty"`
	Price                     float64   `json:"price,omitempty"`
	TriggerPrice              float64   `json:"trigger_price,omitempty"`
	StopLossPrice             float64   `json:"stop_loss_price,omitempty"`
	TakeProfitPrice           float64   `json:"take_profit_price,omitempty"`
	SetStopLossPrice          float64   `json:"set_stop_loss,omitempty"`
	SetTakeProfitPrice        float64   `json:"set_take_profit,omitempty"`
	NominalUSDT               float64   `json:"nominal_usdt,omitempty"`
	MarginAmount              float64   `json:"margin_amount,omitempty"`
	ScaleOutFirst             float64   `json:"scale_out_1st,omitempty"`
	ScaleOutFirstPct          float64   `json:"scale_out_1st_pct,omitempty"`
	PartialTakeProfitOneRatio float64   `json:"partial_take_profit_1_ratio,omitempty"`
	FinalTakeProfit           float64   `json:"final_take_profit,omitempty"`
	Side                      string    `json:"side,omitempty"`
	Comment                   string    `json:"comment,omitempty"`
	ReasonAlt                 string    `json:"reason,omitempty"`
	PositionTag               string    `json:"position,omitempty"`
	PartialTakeProfitOne      float64   `json:"partial_take_profit_1,omitempty"`
	PartialTakeProfitOnePct   float64   `json:"partial_take_profit_1_percent,omitempty"`
	BreakEvenTrigger          float64   `json:"break_even_trigger,omitempty"`
	MarginUsageCurrent        float64   `json:"margin_usage_current,omitempty"`
	MarginUsageTarget         float64   `json:"margin_usage_target,omitempty"`
	OpenPriceRange            []float64 `json:"open_price_range,omitempty"`
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	SystemPrompt string     `json:"system_prompt"` // 系统提示词（发送给AI的系统prompt）
	UserPrompt   string     `json:"user_prompt"`   // 发送给AI的输入prompt
	CoTTrace     string     `json:"cot_trace"`     // 思维链分析（AI输出）
	Decisions    []Decision `json:"decisions"`     // 具体决策列表
	Timestamp    time.Time  `json:"timestamp"`
}

// GetFullDecision 获取AI的完整交易决策（批量分析所有币种和持仓）
func GetFullDecision(ctx *Context, mcpClient *mcp.Client) (*FullDecision, error) {
	return GetFullDecisionWithCustomPrompt(ctx, mcpClient, "", false, "")
}

// GetFullDecisionWithCustomPrompt 获取AI的完整交易决策（支持自定义prompt和模板选择）

const maxAIJSONRetries = 2

func GetFullDecisionWithCustomPrompt(ctx *Context, mcpClient *mcp.Client, customPrompt string, overrideBase bool, templateName string) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	systemPrompt := buildSystemPromptWithCustom(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, customPrompt, overrideBase, templateName)
	baseUserPrompt := buildUserPrompt(ctx)
	var lastErr error

	for attempt := 1; attempt <= maxAIJSONRetries; attempt++ {
		userPrompt := baseUserPrompt
		if attempt > 1 {
			userPrompt += "\n\n⚠️ 上一轮未输出有效的 JSON 决策，请严格使用 <decision> 标签 + 纯 JSON 数组（[{...}])，不要只写思维链。"
		}

		aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
		if err != nil {
			lastErr = fmt.Errorf("调用AI API失败: %w", err)
			continue
		}

		decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.Positions)
		if err != nil {
			lastErr = fmt.Errorf("解析AI响应失败: %w", err)
			continue
		}

		if isFallbackWaitDecision(decision.Decisions) {
			lastErr = fmt.Errorf("AI未输出结构化JSON决策")
			log.Printf("⚠️ AI 输出缺少 JSON (attempt %d/%d)，准备重试", attempt, maxAIJSONRetries)
			continue
		}

		decision.Timestamp = time.Now()
		decision.SystemPrompt = systemPrompt
		decision.UserPrompt = userPrompt
		return decision, nil
	}

	return nil, fmt.Errorf("AI多次输出无效决策: %w", lastErr)
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于阈值的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		// 💡 OI 門檻配置：用戶可根據風險偏好調整
		const minOIThresholdMillions = 15.0 // 可調整：15M(保守) / 10M(平衡) / 8M(寬鬆) / 5M(激進)

		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < minOIThresholdMillions {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < %.1fM)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, minOIThresholdMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// ⚠️ 重要：限制候选币种数量，避免 Prompt 过大
	// 根据持仓数量动态调整：持仓越少，可以分析更多候选币
	const (
		maxCandidatesWhenEmpty    = 30 // 无持仓时最多分析30个候选币
		maxCandidatesWhenHolding1 = 25 // 持仓1个时最多分析25个候选币
		maxCandidatesWhenHolding2 = 20 // 持仓2个时最多分析20个候选币
		maxCandidatesWhenHolding3 = 15 // 持仓3个时最多分析15个候选币（避免 Prompt 过大）
	)

	positionCount := len(ctx.Positions)
	var maxCandidates int

	switch positionCount {
	case 0:
		maxCandidates = maxCandidatesWhenEmpty
	case 1:
		maxCandidates = maxCandidatesWhenHolding1
	case 2:
		maxCandidates = maxCandidatesWhenHolding2
	default: // 3+ 持仓
		maxCandidates = maxCandidatesWhenHolding3
	}

	// 返回实际候选币数量和上限中的较小值
	return min(len(ctx.CandidateCoins), maxCandidates)
}

// buildSystemPromptWithCustom 构建包含自定义内容的 System Prompt
func buildSystemPromptWithCustom(accountEquity float64, btcEthLeverage, altcoinLeverage int, customPrompt string, overrideBase bool, templateName string) string {
	// 如果覆盖基础prompt且有自定义prompt，只使用自定义prompt
	if overrideBase && customPrompt != "" {
		return customPrompt
	}

	// 获取基础prompt（使用指定的模板）
	basePrompt := buildSystemPrompt(accountEquity, btcEthLeverage, altcoinLeverage, templateName)

	// 如果没有自定义prompt，直接返回基础prompt
	if customPrompt == "" {
		return basePrompt
	}

	// 添加自定义prompt部分到基础prompt
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")
	sb.WriteString("# 📌 个性化交易策略\n\n")
	sb.WriteString(customPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("注意: 以上个性化策略是对基础规则的补充，不能违背基础风险控制原则。\n")

	return sb.String()
}

// buildSystemPrompt 构建 System Prompt（使用模板+动态部分）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) string {
	var sb strings.Builder

	// 1. 加载提示词模板（核心交易策略部分）
	if templateName == "" {
		templateName = "default" // 默认使用 default 模板
	}

	template, err := GetPromptTemplate(templateName)
	if err != nil {
		// 如果模板不存在，记录错误并使用 default
		log.Printf("⚠️  提示词模板 '%s' 不存在，使用 default: %v", templateName, err)
		template, err = GetPromptTemplate("default")
		if err != nil {
			// 如果连 default 都不存在，使用内置的简化版本
			log.Printf("❌ 无法加载任何提示词模板，使用内置简化版本")
			sb.WriteString("你是专业的加密货币交易AI。请根据市场数据做出交易决策。\n\n")
		} else {
			sb.WriteString(template.Content)
			sb.WriteString("\n\n")
		}
	} else {
		sb.WriteString(template.Content)
		sb.WriteString("\n\n")
	}

	// 2. 硬约束（风险控制）- 动态生成
	sb.WriteString("# 硬约束（风险控制）\n\n")
	sb.WriteString("1. 风险回报比: 必须 ≥ 1:2；若遇到极强动量或关键位破位，允许在思维链中说明理由后放宽至 ≥1:1.8\n")
	sb.WriteString("2. 最多持仓: 3个币种（质量>数量）\n")

	altMin, altMax := calcPositionRange(accountEquity, 0.4, 0.8, 12, 0.8)

	var btcMin, btcMax float64
	if accountEquity <= 200 {
		btcMin, btcMax = 120, 180
	} else {
		btcMin, btcMax = calcPositionRange(accountEquity, 0.6, 1.2, 105, 2.0)
	}

	sb.WriteString(fmt.Sprintf("3. 单币仓位: 山寨%.0f-%.0f U | BTC/ETH %.0f-%.0f U\n",
		altMin, altMax, btcMin, btcMax))
	sb.WriteString(fmt.Sprintf("4. 杠杆限制: **山寨币最大%dx杠杆** | **BTC/ETH最大%dx杠杆** (⚠️ 严格执行，不可超过)\n", altcoinLeverage, btcEthLeverage))
	sb.WriteString("5. 保证金: 总使用率 ≤ 90%\n")
	sb.WriteString("6. 开仓金额: 建议 **≥12 USDT** (交易所最小名义价值 10 USDT + 安全边际)\n\n")

	// 3. 输出格式 - 动态生成
	sb.WriteString("# 输出格式 (严格遵守)\n\n")
	sb.WriteString("**必须使用XML标签 <reasoning> 和 <decision> 标签分隔思维链和决策JSON，避免解析错误**\n\n")
	sb.WriteString("## 格式要求\n\n")
	sb.WriteString("<reasoning>\n")
	sb.WriteString("你的思维链分析...\n")
	sb.WriteString("- 简洁分析你的思考过程 \n")
	sb.WriteString("</reasoning>\n\n")
	sb.WriteString("<decision>\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"},\n", btcEthLeverage, accountEquity*5))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\", \"reasoning\": \"止盈离场\"}\n")
	sb.WriteString("]\n```\n")
	sb.WriteString("</decision>\n\n")
	sb.WriteString("## 字段说明\n\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString("- `confidence`: 0-100（开仓建议≥75）\n")
	sb.WriteString("- 开仓时必填: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd, reasoning\n\n")

	return sb.String()
}

// calcPositionRange 根据账户规模给出合适的仓位区间，避免小账户出现不可执行的指引
func calcPositionRange(accountEquity, minFactor, maxFactor, absoluteMin, maxCapMultiplier float64) (float64, float64) {
	if accountEquity <= 0 {
		return absoluteMin, absoluteMin * 1.2
	}

	minVal := math.Max(absoluteMin, accountEquity*minFactor)
	maxVal := math.Max(minVal, accountEquity*maxFactor)

	if maxCapMultiplier > 0 {
		maxCap := accountEquity * maxCapMultiplier
		if maxVal > maxCap {
			maxVal = maxCap
		}
	}

	return minVal, maxVal
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("时间: %s | 周期: #%d | 运行: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC 市场
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("BTC: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	sb.WriteString(fmt.Sprintf("账户: 净值%.2f | 余额%.2f (%.1f%%) | 盈亏%+.2f%% | 保证金%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// 持仓（完整市场数据）
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60) // 转换为分钟
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 盈亏%+.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

			// 使用FormatMarketData输出完整市场数据
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData))
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("当前持仓: 无\n\n")
	}

	// 候选币种（完整市场数据）
	sb.WriteString(fmt.Sprintf("## 候选币种 (%d个)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		// 使用FormatMarketData输出完整市场数据
		sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(market.Format(marketData))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 夏普比率（直接传值，不要复杂格式化）
	if ctx.Performance != nil {
		// 直接从interface{}中提取SharpeRatio
		type PerformanceData struct {
			SharpeRatio float64 `json:"sharpe_ratio"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", perfData.SharpeRatio))
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, positions []PositionInfo) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w", err)
	}

	// 3. 兼容并修正不同schema的输出，然后验证
	decisions = normalizeDecisionFields(decisions, accountEquity)
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage, positions); err != nil {
		logDecisionValidationFailure(decisions, aiResponse, err)
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w", err)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析
func extractCoTTrace(response string) string {
	// 方法1: 优先尝试提取 <reasoning> 标签内容
	if match := reReasoningTag.FindStringSubmatch(response); match != nil && len(match) > 1 {
		log.Printf("✓ 使用 <reasoning> 标签提取思维链")
		return strings.TrimSpace(match[1])
	}

	// 方法2: 如果没有 <reasoning> 标签，但有 <decision> 标签，提取 <decision> 之前的内容
	if decisionIdx := strings.Index(response, "<decision>"); decisionIdx > 0 {
		log.Printf("✓ 提取 <decision> 标签之前的内容作为思维链")
		return strings.TrimSpace(response[:decisionIdx])
	}

	// 方法3: 后备方案 - 查找JSON数组的开始位置
	jsonStart := strings.Index(response, "[")
	if jsonStart > 0 {
		log.Printf("⚠️  使用旧版格式（[ 字符分离）提取思维链")
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到任何标记，整个响应都是思维链
	return strings.TrimSpace(response)
}

// extractDecisions 提取JSON决策列表
func extractDecisions(response string) ([]Decision, error) {
	// 预清洗：去零宽/BOM
	s := removeInvisibleRunes(response)
	s = strings.TrimSpace(s)

	// 🔧 关键修复 (Critical Fix)：在正则匹配之前就先修复全角字符！
	// 否则正则表达式 \[ 无法匹配全角的 ［
	s = fixMissingQuotes(s)

	// 方法1: 优先尝试从 <decision> 标签中提取
	var jsonPart string
	if match := reDecisionTag.FindStringSubmatch(s); match != nil && len(match) > 1 {
		jsonPart = strings.TrimSpace(match[1])
		log.Printf("✓ 使用 <decision> 标签提取JSON")
	} else {
		// 后备方案：使用整个响应
		jsonPart = s
		log.Printf("⚠️  未找到 <decision> 标签，使用全文搜索JSON")
	}

	// 修复 jsonPart 中的全角字符
	jsonPart = fixMissingQuotes(jsonPart)

	// 1) 优先从 ```json 代码块中提取
	if m := reJSONFence.FindStringSubmatch(jsonPart); m != nil && len(m) > 1 {
		jsonContent := strings.TrimSpace(m[1])
		jsonContent = compactArrayOpen(jsonContent) // 把 "[ {" 规整为 "[{"
		jsonContent = fixMissingQuotes(jsonContent) // 二次修复（防止 regex 提取后还有残留全角）
		if err := validateJSONFormat(jsonContent); err != nil {
			return nil, fmt.Errorf("JSON格式验证失败: %w\nJSON内容: %s\n完整响应:\n%s", err, jsonContent, response)
		}
		var decisions []Decision
		if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
			return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
		}
		return decisions, nil
	}

	// 2) 退而求其次 (Fallback)：全文寻找首个对象数组
	// 注意：此时 jsonPart 已经过 fixMissingQuotes()，全角字符已转换为半角
	jsonContent := strings.TrimSpace(reJSONArray.FindString(jsonPart))
	if jsonContent == "" {
		// 🔧 安全回退 (Safe Fallback)：当AI只输出思维链没有JSON时，生成保底决策（避免系统崩溃）
		log.Printf("⚠️  [SafeFallback] AI未输出JSON决策，进入安全等待模式 (AI response without JSON, entering safe wait mode)")

		// 提取思维链摘要（最多 240 字符）
		cotSummary := jsonPart
		if len(cotSummary) > 240 {
			cotSummary = cotSummary[:240] + "..."
		}

		// 生成保底决策：所有币种进入 wait 状态
		fallbackDecision := Decision{
			Symbol:    "ALL",
			Action:    "wait",
			Reasoning: fmt.Sprintf("模型未输出结构化JSON决策，进入安全等待；摘要：%s", cotSummary),
		}

		return []Decision{fallbackDecision}, nil
	}

	// 🔧 规整格式（此时全角字符已在前面修复过）
	jsonContent = compactArrayOpen(jsonContent)
	jsonContent = fixMissingQuotes(jsonContent) // 二次修复（防止 regex 提取后还有残留全角）

	// 🔧 验证 JSON 格式（检测常见错误）
	if err := validateJSONFormat(jsonContent); err != nil {
		return nil, fmt.Errorf("JSON格式验证失败: %w\nJSON内容: %s\n完整响应:\n%s", err, jsonContent, response)
	}

	// 解析JSON
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
	}

	return decisions, nil
}

// fixMissingQuotes 替换中文引号和全角字符为英文引号和半角字符（避免AI输出全角JSON字符导致解析失败）
func fixMissingQuotes(jsonStr string) string {
	// 替换中文引号
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '

	// ⚠️ 替换全角括号、冒号、逗号（防止AI输出全角JSON字符）
	jsonStr = strings.ReplaceAll(jsonStr, "［", "[") // U+FF3B 全角左方括号
	jsonStr = strings.ReplaceAll(jsonStr, "］", "]") // U+FF3D 全角右方括号
	jsonStr = strings.ReplaceAll(jsonStr, "｛", "{") // U+FF5B 全角左花括号
	jsonStr = strings.ReplaceAll(jsonStr, "｝", "}") // U+FF5D 全角右花括号
	jsonStr = strings.ReplaceAll(jsonStr, "：", ":") // U+FF1A 全角冒号
	jsonStr = strings.ReplaceAll(jsonStr, "，", ",") // U+FF0C 全角逗号

	// ⚠️ 替换CJK标点符号（AI在中文上下文中也可能输出这些）
	jsonStr = strings.ReplaceAll(jsonStr, "【", "[") // CJK左方头括号 U+3010
	jsonStr = strings.ReplaceAll(jsonStr, "】", "]") // CJK右方头括号 U+3011
	jsonStr = strings.ReplaceAll(jsonStr, "〔", "[") // CJK左龟壳括号 U+3014
	jsonStr = strings.ReplaceAll(jsonStr, "〕", "]") // CJK右龟壳括号 U+3015
	jsonStr = strings.ReplaceAll(jsonStr, "、", ",") // CJK顿号 U+3001

	// ⚠️ 替换全角空格为半角空格（JSON中不应该有全角空格）
	jsonStr = strings.ReplaceAll(jsonStr, "　", " ") // U+3000 全角空格

	return jsonStr
}

// validateJSONFormat 验证 JSON 格式，检测常见错误
func validateJSONFormat(jsonStr string) error {
	trimmed := strings.TrimSpace(jsonStr)

	// 允许 [ 和 { 之间存在任意空白（含零宽）
	if !reArrayHead.MatchString(trimmed) {
		// 检查是否是纯数字/范围数组（常见错误）
		if strings.HasPrefix(trimmed, "[") && !strings.Contains(trimmed[:min(20, len(trimmed))], "{") {
			return fmt.Errorf("不是有效的决策数组（必须包含对象 {}），实际内容: %s", trimmed[:min(50, len(trimmed))])
		}
		return fmt.Errorf("JSON 必须以 [{ 开头（允许空白），实际: %s", trimmed[:min(20, len(trimmed))])
	}

	// 检查是否包含范围符号 ~（LLM 常见错误）
	if containsRangeSymbolOutsideString(jsonStr) {
		return fmt.Errorf("JSON 中不可包含范围符号 ~，所有数字必须是精确的单一值")
	}

	// 检查是否包含千位分隔符（如 98,000）
	// 使用简单的模式匹配：数字+逗号+3位数字
	for i := 0; i < len(jsonStr)-4; i++ {
		if jsonStr[i] >= '0' && jsonStr[i] <= '9' &&
			jsonStr[i+1] == ',' &&
			jsonStr[i+2] >= '0' && jsonStr[i+2] <= '9' &&
			jsonStr[i+3] >= '0' && jsonStr[i+3] <= '9' &&
			jsonStr[i+4] >= '0' && jsonStr[i+4] <= '9' {
			return fmt.Errorf("JSON 数字不可包含千位分隔符逗号，发现: %s", jsonStr[i:min(i+10, len(jsonStr))])
		}
	}

	return nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// removeInvisibleRunes 去除零宽字符和 BOM，避免肉眼看不见的前缀破坏校验
func removeInvisibleRunes(s string) string {
	return reInvisibleRunes.ReplaceAllString(s, "")
}

// compactArrayOpen 规整开头的 "[ {" → "[{"
func compactArrayOpen(s string) string {
	return reArrayOpenSpace.ReplaceAllString(strings.TrimSpace(s), "[{")
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, positions []PositionInfo) error {
	// ⚠️ 累积仓位检查：统计当前持仓中每个标的每个方向的数量
	positionCounts := make(map[string]int) // key: "BTCUSDT_long" or "BTCUSDT_short"

	for _, pos := range positions {
		key := fmt.Sprintf("%s_%s", pos.Symbol, pos.Side)
		positionCounts[key]++
	}

	// 检查每个开仓决策
	for i := range decisions {
		d := &decisions[i]

		// 只检查开仓操作
		if d.Action == "open_long" || d.Action == "open_short" {
			direction := "long"
			if d.Action == "open_short" {
				direction = "short"
			}
			key := fmt.Sprintf("%s_%s", d.Symbol, direction)
			currentCount := positionCounts[key]

			// ⚠️ 禁止同向累积超过2次
			if currentCount >= 2 {
				return fmt.Errorf("决策 #%d 违反累积仓位限制: %s 已有 %d 个%s仓位，禁止第3次加仓（风控规则：同向最多2个仓位）",
					i+1, d.Symbol, currentCount, direction)
			}

			// ⚠️ 禁止多空对冲：检查是否存在反向仓位
			oppositeDirection := "short"
			if direction == "short" {
				oppositeDirection = "long"
			}
			oppositeKey := fmt.Sprintf("%s_%s", d.Symbol, oppositeDirection)
			if positionCounts[oppositeKey] > 0 {
				return fmt.Errorf("决策 #%d 违反多空对冲禁止规则: %s 已有 %d 个%s仓位，禁止开%s仓（必须先平掉反向仓位）",
					i+1, d.Symbol, positionCounts[oppositeKey], oppositeDirection, direction)
			}

			// 如果这个决策通过检查，将它加入到计数中（用于检查后续决策）
			positionCounts[key]++
		}

		// 验证单个决策的其他规则
		if err := validateDecision(d, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":          true,
		"open_short":         true,
		"close_long":         true,
		"close_short":        true,
		"update_stop_loss":   true,
		"update_take_profit": true,
		"partial_close":      true,
		"hold":               true,
		"wait":               true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage           // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 10.0 // 山寨币最多10倍账户净值
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}

		// ✅ 验证最小开仓金额（根据交易所规则动态调整）
		minPositionSize := util.GetSafeMinPositionUSD(d.Symbol)
		symbolUpper := strings.ToUpper(d.Symbol)
		if d.PositionSizeUSD < minPositionSize {
			// 自动修正到最小值，而不是拒绝
			oldSize := d.PositionSizeUSD
			d.PositionSizeUSD = minPositionSize
			log.Printf("⚠️  自动修正仓位: %s 原始值 %.2f USDT → 修正为 %.2f USDT（最小开仓要求）", symbolUpper, oldSize, minPositionSize)
		}

		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			// 自动修正到账户净值上限，而不是拒绝
			oldSize := d.PositionSizeUSD
			d.PositionSizeUSD = maxPositionValue
			log.Printf("⚠️  自动修正仓位: %s 原始值 %.2f USDT → 修正为 %.2f USDT（账户净值上限=%.2f×10）", symbolUpper, oldSize, maxPositionValue, accountEquity)
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 硬约束：风险回报比必须≥3.0
		if riskRewardRatio < 3.0 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥3.0:1 [风险:%.2f%% 收益:%.2f%%] [止损:%.2f 止盈:%.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	// 动态调整止损验证
	if d.Action == "update_stop_loss" {
		if d.NewStopLoss <= 0 && d.StopLoss > 0 {
			d.NewStopLoss = d.StopLoss // 兼容旧字段命名
		}
		if d.NewStopLoss <= 0 {
			return fmt.Errorf("新止损价格必须大于0: %.2f", d.NewStopLoss)
		}
	}

	// 动态调整止盈验证
	if d.Action == "update_take_profit" {
		if d.NewTakeProfit <= 0 && d.TakeProfit > 0 {
			d.NewTakeProfit = d.TakeProfit // 兼容旧字段命名
		}
		if d.NewTakeProfit <= 0 {
			return fmt.Errorf("新止盈价格必须大于0: %.2f", d.NewTakeProfit)
		}
	}

	// 部分平仓验证
	if d.Action == "partial_close" {
		if d.ClosePercentage <= 0 || d.ClosePercentage > 100 {
			return fmt.Errorf("平仓百分比必须在0-100之间: %.1f", d.ClosePercentage)
		}
	}

	return nil
}

func isFallbackWaitDecision(decisions []Decision) bool {
	if len(decisions) != 1 {
		return false
	}
	d := decisions[0]
	if !strings.EqualFold(d.Action, "wait") {
		return false
	}
	if !strings.EqualFold(d.Symbol, "ALL") {
		return false
	}
	return strings.Contains(d.Reasoning, "模型未输出结构化JSON决策")
}

func logDecisionValidationFailure(decisions []Decision, aiResponse string, validateErr error) {
	log.Printf("❌ 决策验证失败: %v", validateErr)

	if len(decisions) > 0 {
		if data, err := json.MarshalIndent(decisions, "", "  "); err == nil {
			log.Printf("🧾 AI决策JSON内容:\n%s", data)
		} else {
			log.Printf("⚠️  无法序列化AI决策JSON: %v", err)
		}
	} else {
		log.Printf("⚠️  AI输出未包含任何决策")
	}

	log.Printf("🧠 AI原始响应(截断):\n%s", truncateString(aiResponse, 4000))
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "...(truncated)"
}

func normalizeDecisionFields(decisions []Decision, accountEquity float64) []Decision {
	normalized := make([]Decision, 0, len(decisions))
	for _, d := range decisions {
		lowerAction := strings.ToLower(strings.TrimSpace(d.Action))

		if d.Percentage <= 0 && d.PercentAlias > 0 {
			d.Percentage = d.PercentAlias
		}

		switch lowerAction {
		case "":
			if strings.TrimSpace(d.ReasonAlt) != "" || strings.TrimSpace(d.Reasoning) != "" {
				d.Action = "wait"
			}
		case "open_limit":
			if strings.EqualFold(d.Side, "sell") {
				d.Action = "open_short"
			} else {
				d.Action = "open_long"
			}
		case "open_market":
			if strings.EqualFold(d.Side, "sell") {
				d.Action = "open_short"
			} else {
				d.Action = "open_long"
			}
		case "set_stop_loss":
			d.Action = "update_stop_loss"
		case "set_take_profit":
			d.Action = "update_take_profit"
		case "close_position", "closepos", "close_positions":
			if strings.EqualFold(d.Side, "sell") || strings.EqualFold(d.Side, "short") ||
				strings.Contains(strings.ToLower(d.Comment), "short") {
				d.Action = "close_short"
			} else {
				d.Action = "close_long"
			}
		case "reduce", "reduce_position", "partial_reduce", "scale_out", "close_position_partial":
			d.Action = "partial_close"
		case "set_stop", "set_stoploss":
			d.Action = "update_stop_loss"
		case "wait":
			d.Action = "wait"
		case "set_position", "hold_position", "maintain_position":
			d.Action = "hold"
		}

		if d.Reasoning == "" && d.Comment != "" {
			d.Reasoning = d.Comment
		}
		if d.AmountType == "" && d.Percentage > 0 {
			d.AmountType = "percent"
			d.AmountValue = d.Percentage
		}

		switch d.Action {
		case "open_long", "open_short":
			normalizeOpenDecision(&d, accountEquity)
		case "update_stop_loss":
			if d.NewStopLoss <= 0 {
				if d.StopLossPrice > 0 {
					d.NewStopLoss = d.StopLossPrice
				} else if d.SetStopLossPrice > 0 {
					d.NewStopLoss = d.SetStopLossPrice
				} else if d.Price > 0 {
					d.NewStopLoss = d.Price
				}
			}
		case "update_take_profit":
			if d.NewTakeProfit <= 0 {
				if d.TakeProfitPrice > 0 {
					d.NewTakeProfit = d.TakeProfitPrice
				} else if d.SetTakeProfitPrice > 0 {
					d.NewTakeProfit = d.SetTakeProfitPrice
				} else if d.ScaleOutFirst > 0 {
					d.NewTakeProfit = d.ScaleOutFirst
				} else if d.Price > 0 {
					d.NewTakeProfit = d.Price
				}
			}
		case "close_long", "close_short":
			if d.Percentage > 0 && d.Percentage < 100 {
				d.ClosePercentage = d.Percentage
				d.Action = "partial_close"
			}
		case "partial_close":
			ensureClosePercentage(&d)
		}

		normalized = append(normalized, d)
	}
	return normalized
}

func normalizeOpenDecision(d *Decision, accountEquity float64) {
	// entry price (用于推导止盈等)
	entryPrice := d.EntryPrice
	if entryPrice <= 0 {
		entryPrice = d.LimitPrice
	}
	if entryPrice <= 0 {
		entryPrice = d.OpenPrice
	}
	if entryPrice <= 0 && len(d.OpenPriceRange) > 0 {
		if len(d.OpenPriceRange) == 1 {
			entryPrice = d.OpenPriceRange[0]
		} else {
			entryPrice = averageRange(d.OpenPriceRange)
		}
	}
	if entryPrice <= 0 {
		entryPrice = d.TriggerPrice
	}
	if entryPrice <= 0 {
		entryPrice = d.Price
	}

	// position size
	if d.PositionSizeUSD <= 0 {
		if d.MarginAmount > 0 {
			lev := d.Leverage
			if lev <= 0 {
				lev = 1
			}
			d.PositionSizeUSD = d.MarginAmount * float64(lev)
		}
		if d.NominalUSDT > 0 {
			d.PositionSizeUSD = d.NominalUSDT
		}
		if d.PositionSizeUSD <= 0 && d.Quantity > 0 && entryPrice > 0 {
			d.PositionSizeUSD = d.Quantity * entryPrice
		} else {
			leverage := d.Leverage
			if leverage <= 0 {
				leverage = 1
			}
			switch strings.ToLower(d.AmountType) {
			case "percent":
				d.PositionSizeUSD = accountEquity * (d.AmountValue / 100.0) * float64(leverage)
			case "usd", "margin":
				d.PositionSizeUSD = d.AmountValue * float64(leverage)
			case "notional":
				d.PositionSizeUSD = d.AmountValue
			}
			if d.PositionSizeUSD <= 0 && d.AmountValue > 0 {
				d.PositionSizeUSD = d.AmountValue
			}
		}
	}
	if d.PositionSizeUSD > 0 {
		minPosition := util.GetSafeMinPositionUSD(strings.ToUpper(d.Symbol))
		if minPosition > 0 && d.PositionSizeUSD < minPosition {
			d.PositionSizeUSD = minPosition
		}
		maxPosition := accountEquity * 10
		if strings.EqualFold(d.Symbol, "BTCUSDT") || strings.EqualFold(d.Symbol, "ETHUSDT") {
			maxPosition = accountEquity * 10
		}
		if d.PositionSizeUSD > maxPosition {
			d.PositionSizeUSD = maxPosition
		}
	}

	// stop loss
	if d.StopLoss <= 0 {
		if d.StopLossPrice > 0 {
			d.StopLoss = d.StopLossPrice
		} else if d.SetStopLossPrice > 0 {
			d.StopLoss = d.SetStopLossPrice
		} else if entryPrice > 0 {
			offset := entryPrice * 0.004 // 默认0.4%缓冲
			if d.Action == "open_long" {
				d.StopLoss = entryPrice - offset
			} else {
				d.StopLoss = entryPrice + offset
			}
		}
	}

	// take profit
	if d.TakeProfit <= 0 {
		if d.TakeProfitPrice > 0 {
			d.TakeProfit = d.TakeProfitPrice
		} else if d.SetTakeProfitPrice > 0 {
			d.TakeProfit = d.SetTakeProfitPrice
		} else if d.ScaleOutFirst > 0 {
			d.TakeProfit = d.ScaleOutFirst
		} else if d.PartialTakeProfitOne > 0 {
			d.TakeProfit = d.PartialTakeProfitOne
		} else if d.FinalTakeProfit > 0 {
			d.TakeProfit = d.FinalTakeProfit
		} else if entryPrice > 0 && d.StopLoss > 0 {
			diff := math.Abs(entryPrice - d.StopLoss)
			if diff > 0 {
				if d.Action == "open_long" {
					d.TakeProfit = entryPrice + diff*3
				} else {
					d.TakeProfit = entryPrice - diff*3
				}
			}
		}
	}

	// risk usd estimation if missing
	if d.RiskUSD <= 0 && d.StopLoss > 0 && entryPrice > 0 && d.PositionSizeUSD > 0 && d.Leverage > 0 {
		margin := d.PositionSizeUSD / float64(d.Leverage)
		priceDiffPct := math.Abs(entryPrice-d.StopLoss) / entryPrice
		d.RiskUSD = margin * priceDiffPct
	}

	if d.Reasoning == "" && d.ReasonAlt != "" {
		d.Reasoning = d.ReasonAlt
	}
}

func averageRange(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	count := 0
	for _, v := range values {
		if v == 0 {
			continue
		}
		sum += v
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func ensureClosePercentage(d *Decision) {
	if d.ClosePercentage <= 0 {
		switch strings.ToLower(d.AmountType) {
		case "percent", "percentage":
			if d.AmountValue > 0 {
				d.ClosePercentage = d.AmountValue
			}
		}
	}
	if d.ClosePercentage <= 0 && d.Percentage > 0 {
		d.ClosePercentage = d.Percentage
	}
	if d.ClosePercentage <= 0 && d.PartialTakeProfitOnePct > 0 {
		d.ClosePercentage = d.PartialTakeProfitOnePct
	}
	if d.ClosePercentage <= 0 && d.PartialTakeProfitOneRatio > 0 {
		d.ClosePercentage = d.PartialTakeProfitOneRatio * 100
	}
	if d.ClosePercentage <= 0 {
		d.ClosePercentage = 100
	}
	if d.ClosePercentage > 100 {
		d.ClosePercentage = 100
	}
}

func containsRangeSymbolOutsideString(s string) bool {
	inString := false
	escape := false
	for _, r := range s {
		if escape {
			escape = false
			continue
		}
		if r == '\\' && inString {
			escape = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '~' && !inString {
			return true
		}
	}
	return false
}
