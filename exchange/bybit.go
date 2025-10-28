package exchange

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/hirokisan/bybit/v2"
)

type Bybit struct {
	client   *bybit.Client
	wsClient *bybit.WebSocketClient
}

func NewBybit(key, secret string) *Bybit {

	client := bybit.NewClient().WithAuth(key, secret)
	wsClient := bybit.NewWebsocketClient().WithAuth(key, secret)
	return &Bybit{
		client:   client,
		wsClient: wsClient,
	}
}

func (b *Bybit) GetPositionInfo() ([]Position, error) {

	settleCoin := bybit.CoinUSDT
	param := bybit.V5GetPositionInfoParam{
		Category:   bybit.CategoryV5Linear,
		SettleCoin: &settleCoin,
	}

	resp, err := b.client.V5().Position().GetPositionInfo(param)
	if err != nil {
		return nil, fmt.Errorf("[bybit] GetPositionInfo error: %v", err)
	}

	positions := make([]Position, 0)

	for _, posEx := range resp.Result.List {

		pos := Position{}
		pos.Symbol = string(posEx.Symbol)
		pos.Side = string(posEx.Side)
		pos.Size, _ = strconv.ParseFloat(posEx.Size, 64)
		pos.EntryPrice, _ = strconv.ParseFloat(posEx.AvgPrice, 64)
		pos.UnrealisedPnl, _ = strconv.ParseFloat(posEx.UnrealisedPnl, 64)
		pos.CreatedTime = posEx.CreatedTime
		positions = append(positions, pos)
	}
	return positions, nil
}

func (b *Bybit) SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error)) error {

	svc, err := b.wsClient.V5().Private()
	if err != nil {
		return fmt.Errorf("[bybit] failed to create private WS client: %w", err)
	}

	if err := svc.Subscribe(); err != nil {
		return fmt.Errorf("[bybit] failed to subscribe WS: %w", err)
	}

	unsubscribe, err := svc.SubscribePosition(func(msg bybit.V5WebsocketPrivatePositionResponse) error {

		for _, posEx := range msg.Data {

			pos := Position{}
			pos.Symbol = string(posEx.Symbol)
			pos.Side = string(posEx.Side)
			pos.Size, _ = strconv.ParseFloat(posEx.Size, 64)
			pos.EntryPrice, _ = strconv.ParseFloat(posEx.EntryPrice, 64)
			pos.UnrealisedPnl, _ = strconv.ParseFloat(posEx.UnrealisedPnl, 64)
			pos.CumRealisedPnl, _ = strconv.ParseFloat(posEx.CumRealisedPnl, 64)
			pos.CreatedTime = posEx.CreatedTime

			onData(pos)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe position: %w", err)
	}
	// Обработчик ошибок WebSocket
	errHandler := func(isWebsocketClosed bool, err error) {
		if onError != nil {
			onError(fmt.Errorf("[bybit] [SubscribePosition] websocket error (closed=%v): %w", isWebsocketClosed, err))
		}

		if isWebsocketClosed {
			log.Println("[bybit] [SubscribePosition] WebSocket closed, need to reconnect manually")
		}
	}

	// Запускаем WebSocket клиент
	go func() {
		if err := svc.Start(ctx, errHandler); err != nil {
			if onError != nil {
				onError(fmt.Errorf("[bybit] [SubscribePosition] websocket stopped: %w", err))
			}
		}
	}()

	// Отписка
	_ = unsubscribe

	return nil
}

func (b *Bybit) SubscribeTickerStart(ctx context.Context, symbol string, onData func(price float64), onError func(err error)) (func() error, error) {

	pubSvc, err := b.wsClient.V5().Public(bybit.CategoryV5Linear)
	if err != nil {
		return nil, fmt.Errorf("[bybit] failed to create public WS client: %w", err)
	}

	unsubscribe, err := pubSvc.SubscribeTicker(bybit.V5WebsocketPublicTickerParamKey{Symbol: bybit.SymbolV5(symbol)}, func(msg bybit.V5WebsocketPublicTickerResponse) error {

		if msg.Data.LinearInverse.LastPrice != "" {
			price, _ := strconv.ParseFloat(msg.Data.LinearInverse.LastPrice, 64)
			onData(price)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("[bybit] failed to subscribe ticker: %w", err)
	}

	errHandler := func(isWebsocketClosed bool, err error) {
		if onError != nil {
			onError(fmt.Errorf("[bybit] [SubscribeTicker] websocket error (closed=%v): %w", isWebsocketClosed, err))
		}

		if isWebsocketClosed {
			log.Println("[bybit] [SubscribeTicker] WebSocket closed, need to reconnect manually")
		}
	}

	go func() {
		if err := pubSvc.Start(ctx, errHandler); err != nil {
			if onError != nil {
				onError(fmt.Errorf("[bybit] [SubscribeTicker] websocket stopped: %w", err))
			}
		}
	}()

	return unsubscribe, nil
}

func V5WebsocketPrivatePositionDataString(posEx bybit.V5WebsocketPrivatePositionData) {
	fmt.Printf("┌─────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│                 POSITION DETAILS (V5)                  │\n")
	fmt.Printf("├─────────────────────────────────────────────────────────┤\n")
	fmt.Printf("│ %-20s: %-30s │\n", "Symbol", posEx.Symbol)
	fmt.Printf("│ %-20s: %-30s │\n", "Side", posEx.Side)
	fmt.Printf("│ %-20s: %-30s │\n", "Category", posEx.Category)
	fmt.Printf("│ %-20s: %-30s │\n", "Size", posEx.Size)
	fmt.Printf("│ %-20s: %-30s │\n", "Entry Price", posEx.EntryPrice)
	fmt.Printf("│ %-20s: %-30s │\n", "Mark Price", posEx.MarkPrice)
	fmt.Printf("│ %-20s: %-30s │\n", "Leverage", posEx.Leverage)
	fmt.Printf("│ %-20s: %-30s │\n", "Unrealised PnL", posEx.UnrealisedPnl)
	fmt.Printf("│ %-20s: %-30s │\n", "Cum Realised PnL", posEx.CumRealisedPnl)
	fmt.Printf("│ %-20s: %-30s │\n", "Position Value", posEx.PositionValue)
	fmt.Printf("│ %-20s: %-30s │\n", "Position Balance", posEx.PositionBalance)
	fmt.Printf("├─────────────────────────────────────────────────────────┤\n")
	fmt.Printf("│ %-20s: %-30s │\n", "Take Profit", posEx.TakeProfit)
	fmt.Printf("│ %-20s: %-30s │\n", "Stop Loss", posEx.StopLoss)
	fmt.Printf("│ %-20s: %-30s │\n", "Trailing Stop", posEx.TrailingStop)
	fmt.Printf("│ %-20s: %-30s │\n", "TPSL Mode", posEx.TpslMode)
	fmt.Printf("├─────────────────────────────────────────────────────────┤\n")
	fmt.Printf("│ %-20s: %-30s │\n", "Liquidation Price", posEx.LiqPrice)
	fmt.Printf("│ %-20s: %-30s │\n", "Bust Price", posEx.BustPrice)
	fmt.Printf("│ %-20s: %-30s │\n", "Position IM", posEx.PositionIM)
	fmt.Printf("│ %-20s: %-30s │\n", "Position MM", posEx.PositionMM)
	fmt.Printf("├─────────────────────────────────────────────────────────┤\n")
	fmt.Printf("│ %-20s: %-30d │\n", "Auto Add Margin", posEx.AutoAddMargin)
	fmt.Printf("│ %-20s: %-30d │\n", "Position Index", posEx.PositionIdx)
	fmt.Printf("│ %-20s: %-30d │\n", "Trade Mode", posEx.TradeMode)
	fmt.Printf("│ %-20s: %-30d │\n", "Risk ID", posEx.RiskID)
	fmt.Printf("│ %-20s: %-30s │\n", "Risk Limit Value", posEx.RiskLimitValue)
	fmt.Printf("│ %-20s: %-30s │\n", "Position Status", posEx.PositionStatus)
	fmt.Printf("├─────────────────────────────────────────────────────────┤\n")
	fmt.Printf("│ %-20s: %-30s │\n", "Created Time", posEx.CreatedTime)
	fmt.Printf("│ %-20s: %-30s │\n", "Updated Time", posEx.UpdatedTime)
	fmt.Printf("└─────────────────────────────────────────────────────────┘\n")
}
