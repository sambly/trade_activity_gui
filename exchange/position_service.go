package exchange

import (
	"context"
	"fmt"
	"log/slog"
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
	log      *slog.Logger
	exchange Exchange
	position map[string]*Position

	mu sync.RWMutex
	// Подписчки на тикеры с функцией отписки
	subscribedTickers map[string]func() error
}

func NewPositionService(exchange Exchange, logger *slog.Logger) *PositionService {
	return &PositionService{
		wg:                &sync.WaitGroup{},
		log:               logger.With("component", "PositionService"),
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
	ps.log.Debug("position added", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)
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
		ps.log.Debug("ticker not subscribed, skipping unsubscribe", "symbol", symbol)
		return
	}

	if err := ps.subscribedTickers[symbol](); err != nil {
		ps.log.Warn("failed to unsubscribe ticker", "symbol", symbol, "error", err)
	} else {
		ps.log.Info("ticker unsubscribed", "symbol", symbol)
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

// Подписка на обновления позиций
func (ps *PositionService) SubscribePositionStart(ctx context.Context) error {
	if ps.exchange == nil {
		return fmt.Errorf("exchange client is not initialized")
	}

	ps.log.Info("starting position subscription")

	dataHandler := func(pos Position) {

		ps.mu.Lock()
		defer ps.mu.Unlock()

		//Позиция закрыта  // TODO пока пытаюсь уловить закрытие по этим состояним, надо анализировать
		if pos.Size == 0 && pos.CumRealisedPnl != 0 {
			delete(ps.position, pos.Symbol)
			go ps.DeleteSubscribeTicker(pos.Symbol)
			ps.log.Info("position closed", "symbol", pos.Symbol, "realised_pnl", pos.CumRealisedPnl)
			return
		}

		if pos.Size != 0 && pos.Side != "" {
			// Обновляем или добавляем позицию
			existing, exists := ps.position[pos.Symbol]
			if exists {
				existing.Side = pos.Side
				existing.Size = pos.Size
				existing.EntryPrice = pos.EntryPrice

				ps.log.Debug("position updated", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)

			} else {
				ps.position[pos.Symbol] = &Position{
					Symbol:        pos.Symbol,
					CreatedTime:   pos.CreatedTime,
					Side:          pos.Side,
					Size:          pos.Size,
					EntryPrice:    pos.EntryPrice,
					UnrealisedPnl: pos.UnrealisedPnl,
				}

				ps.log.Info("new position detected", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)

				go func() {
					if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
						ps.log.Error("failed to subscribe to ticker for new position", "symbol", pos.Symbol, "error", err)
					} else {
						ps.log.Info("ticker subscription started for new position", "symbol", pos.Symbol)
					}
				}()
			}
		}
	}

	errHandler := func(err error) {
		ps.log.Error("position stream error", "error", err)
	}

	if err := ps.exchange.SubscribePositionStart(ctx, dataHandler, errHandler); err != nil {
		ps.log.Error("failed to start position subscription", "error", err)
		return fmt.Errorf("failed to subscribe position stream: %w", err)
	}
	ps.log.Info("position stream subscription started successfully")
	return nil
}

// Подписка на обновления тикеров для обновления цены
func (ps *PositionService) SubscribeTickerStart(ctx context.Context, symbol string) (func() error, error) {
	if ps.exchange == nil {
		return nil, fmt.Errorf("exchange client is not initialized")
	}

	ps.wg.Add(1)

	ps.log.Debug("starting ticker subscription", "symbol", symbol)

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
		}

		existing.UnrealisedPnl = unrealisedPnL
	}

	errHandler := func(err error) {
		ps.log.Error("ticker stream error", "symbol", symbol, "error", err)
	}

	unsubscribe, err := ps.exchange.SubscribeTickerStart(ctx, symbol, dataHandler, errHandler)
	if err != nil {
		ps.wg.Done()
		ps.log.Error("failed to subscribe to ticker", "symbol", symbol, "error", err)
		return nil, fmt.Errorf("failed to subscribe ticker %s: %w", symbol, err)
	}

	wrappedUnsubscribe := func() error {
		defer ps.wg.Done()

		if err := unsubscribe(); err != nil {
			ps.log.Error("failed to unsubscribe from ticker", "symbol", symbol, "error", err)
			return fmt.Errorf("failed to unsubscribe ticker %s: %w", symbol, err)
		}

		ps.log.Info("unsubscribed ticker successfully", "symbol", symbol)
		return nil
	}

	ps.log.Info("ticker subscription started", "symbol", symbol)

	return wrappedUnsubscribe, nil
}

func (ps *PositionService) StopAllSubscriptions() error {

	ps.log.Info("stopping all subscriptions", "ticker_count", len(ps.subscribedTickers))

	for symbol, unsub := range ps.subscribedTickers {
		if err := unsub(); err != nil {
			ps.log.Error("failed to unsubscribe ticker", "symbol", symbol, "error", err)
		}
	}
	ps.wg.Wait()
	ps.log.Info("all subscriptions stopped")
	return nil
}
