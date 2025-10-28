package exchange

import (
	"context"
	"fmt"
	"log"
	"sync"
)

type Position struct {
	Symbol         string
	CreatedTime    string
	Side           string
	Size           float64
	EntryPrice     float64
	UnrealisedPnl  float64
	CumRealisedPnl float64
	CurrentPrice   float64
	CurrentValue   float64
}

type PositionService struct {
	wg       *sync.WaitGroup
	exchange Exchange
	position map[string]*Position

	mu sync.RWMutex
	// Подписчки на тикеры с функцией отписки
	subscribedTickers map[string]func() error
}

func NewPositionService(exchange Exchange) *PositionService {
	return &PositionService{
		wg:                &sync.WaitGroup{},
		exchange:          exchange,
		position:          make(map[string]*Position),
		subscribedTickers: make(map[string]func() error),
	}
}

func (ps *PositionService) GetAllPosition() []Position {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	positions := make([]Position, 0, len(ps.position))
	for _, pos := range ps.position {
		positions = append(positions, *pos)
	}
	return positions
}

func (ps *PositionService) GetPosition(symbol string) (Position, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	pos, exists := ps.position[symbol]
	return *pos, exists
}

func (ps *PositionService) AddPosition(pos *Position) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.position[pos.Symbol] = pos
}

func (ps *PositionService) AddSubscribeTicker(ctx context.Context, symbol string) error {
	if _, ok := ps.subscribedTickers[symbol]; ok {
		return nil
	}
	unsubscribe, err := ps.SubscribeTickerStart(ctx, symbol)
	if err != nil {
		return err
	}
	ps.subscribedTickers[symbol] = unsubscribe
	return nil
}

func (ps *PositionService) DeleteSubscribeTicker(symbol string) {

	if _, ok := ps.subscribedTickers[symbol]; !ok {
		return
	}

	if err := ps.subscribedTickers[symbol](); err != nil {
		log.Printf("⚠️ Failed to unsubscribe ticker %s: %v", symbol, err)
	}
	delete(ps.subscribedTickers, symbol)

}

// Init загружает стартовые позиции из REST API
func (ps *PositionService) Init() error {

	position, err := ps.exchange.GetPositionInfo()
	if err != nil {
		return err
	}
	for _, pos := range position {
		newPos := &Position{
			Symbol:        pos.Symbol,
			CreatedTime:   pos.CreatedTime,
			Side:          pos.Side,
			Size:          pos.Size,
			EntryPrice:    pos.EntryPrice,
			UnrealisedPnl: pos.UnrealisedPnl,
		}
		ps.AddPosition(newPos)
	}
	return nil
}

// SubscribePositionStart запускает подписку на обновления позиций по WebSocket
func (ps *PositionService) SubscribePositionStart(ctx context.Context) error {
	if ps.exchange == nil {
		return fmt.Errorf("exchange client is not initialized")
	}
	dataHandler := func(pos Position) {

		ps.mu.Lock()
		defer ps.mu.Unlock()

		log.Printf("📊 Updated data: %s %s size=%.4f CumRealisedPnl=%.4f", pos.Symbol, pos.Side, pos.Size, pos.CumRealisedPnl)

		//Позиция закрыта  // TODO пока пытаюсь уловить закрытие по этим состояним, надо анализировать
		if pos.Size == 0 && pos.CumRealisedPnl != 0 {
			delete(ps.position, pos.Symbol)
			go ps.DeleteSubscribeTicker(pos.Symbol) // отписываемся от тикера
			log.Printf("❌ Position closed and unsubscribed: %s", pos.Symbol)
			return
		}

		if pos.Size != 0 && pos.Side != "" {
			log.Printf("📊 Updated position: %s %s size=%.4f", pos.Symbol, pos.Side, pos.Size)
			// Обновляем или добавляем позицию
			existing, exists := ps.position[pos.Symbol]
			if exists {
				existing.Side = pos.Side
				existing.Size = pos.Size
				existing.EntryPrice = pos.EntryPrice
			} else {
				ps.position[pos.Symbol] = &Position{
					Symbol:        pos.Symbol,
					CreatedTime:   pos.CreatedTime,
					Side:          pos.Side,
					Size:          pos.Size,
					EntryPrice:    pos.EntryPrice,
					UnrealisedPnl: pos.UnrealisedPnl,
				}
				go func() {
					if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
						log.Printf("➕ ❌ Error adding position & subscribing to ticker %s: %v", pos.Symbol, err)
					} else {
						log.Printf("➕ ✅ Successfully added position & subscribed to ticker: %s", pos.Symbol)
					}
				}()

			}
		}

	}

	errHandler := func(err error) {
		log.Printf("⚠️ Position stream error: %v", err)
	}

	if err := ps.exchange.SubscribePositionStart(ctx, dataHandler, errHandler); err != nil {
		return fmt.Errorf("failed to subscribe position stream: %w", err)
	}

	log.Println("✅ Position stream subscription started")
	return nil
}

func (ps *PositionService) SubscribeTickerStart(ctx context.Context, symbol string) (func() error, error) {
	if ps.exchange == nil {
		return nil, fmt.Errorf("[Ticker] exchange client is not initialized")
	}

	ps.wg.Add(1)

	dataHandler := func(price float64) {

		ps.mu.Lock()
		defer ps.mu.Unlock()

		existing, exists := ps.position[symbol]
		if !exists {
			return
		}
		existing.CurrentPrice = price
		existing.CurrentValue = price * existing.Size

		var unrealisedPnL float64
		switch existing.Side {
		case "Buy":
			unrealisedPnL = (price - existing.EntryPrice) * existing.Size
		case "Sell":
			unrealisedPnL = (existing.EntryPrice - price) * existing.Size
		default:
			unrealisedPnL = 0
			log.Printf("[Ticker] Unknown position side: %s", existing.Side)
		}

		existing.UnrealisedPnl = unrealisedPnL

		//log.Printf("[Ticker] %s UnrealisedPnL: %.6f | Size: %.2f | LastPrice: %.2f", symbol, unrealisedPnL, existing.Size, price)
	}

	errHandler := func(err error) {
		log.Printf("⚠️ [Ticker] websocket error: %v", err)
	}

	unsubscribe, err := ps.exchange.SubscribeTickerStart(ctx, symbol, dataHandler, errHandler)
	if err != nil {
		ps.wg.Done()
		return nil, fmt.Errorf("[Ticker] failed to subscribe ticker: %w", err)
	}

	wrappedUnsubscribe := func() error {
		defer ps.wg.Done()

		if err := unsubscribe(); err != nil {
			return fmt.Errorf("[Ticker] failed to unsubscribe ticker %s: %w", symbol, err)
		}

		log.Printf("[Ticker] %s unsubscribed successfully", symbol)
		return nil
	}

	return wrappedUnsubscribe, nil
}

func (ps *PositionService) StopAllSubscriptions() error {

	for symbol, unsub := range ps.subscribedTickers {
		if err := unsub(); err != nil {
			log.Printf("⚠️ Failed to unsubscribe %s: %v", symbol, err)
		}
	}
	ps.wg.Wait()
	return nil
}
