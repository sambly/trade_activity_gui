package exchange

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"trade_activity_gui/hub"
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
	hub      *hub.Hub

	mu       sync.RWMutex
	position map[string]*Position

	// Подписки на тикеры с функцией отписки
	subscribedTickers  map[string]func() error
	subscribedPosition func() error

	positionStreamErrorCritical chan error
	tickerStreamErrorCritical   chan error
}

func NewPositionService(exchange Exchange, logger *slog.Logger, hub *hub.Hub) *PositionService {
	return &PositionService{
		wg:                          &sync.WaitGroup{},
		log:                         logger.With("component", "PositionService"),
		exchange:                    exchange,
		hub:                         hub,
		position:                    make(map[string]*Position),
		subscribedTickers:           make(map[string]func() error),
		positionStreamErrorCritical: make(chan error, 1),
		tickerStreamErrorCritical:   make(chan error, 1),
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

func (ps *PositionService) AddPosition(pos *Position) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.position[pos.Symbol] = pos
	ps.log.Info("position added", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)
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
	ps.subscribedTickers[symbol]()
	delete(ps.subscribedTickers, symbol)
}

func (ps *PositionService) Init(ctx context.Context) error {

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

	for _, pos := range ps.position {
		if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
			return fmt.Errorf("failed subscribe to ticker %s: %W", pos.Symbol, err)
		}
	}

	return nil
}

func (ps *PositionService) Start(ctx context.Context) error {
	if ps.exchange == nil {
		return fmt.Errorf("exchange client is not initialized")
	}

	if err := ps.Init(ctx); err != nil {
		return fmt.Errorf("init positions: %w", err)
	}

	unsubscribe, err := ps.SubscribePositionStart(ctx)
	if err != nil {
		return err
	}
	ps.subscribedPosition = unsubscribe

	select {
	case <-ctx.Done():
		return nil
	case err := <-ps.positionStreamErrorCritical:
		return err
	case err := <-ps.tickerStreamErrorCritical:
		return err
	}
}

func (ps *PositionService) SubscribePositionStart(ctx context.Context) (func() error, error) {

	serviceName := "SubscribePosition"
	ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
	var connectionStatus hub.ConnectionStatus = hub.Connected

	dataHandler := func(pos Position) {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		if connectionStatus == hub.Disconnected {
			ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
			connectionStatus = hub.Connected
		}

		// Удаление позиции
		if pos.Size == 0 && pos.CumRealisedPnl != 0 {
			delete(ps.position, pos.Symbol)
			ps.DeleteSubscribeTicker(pos.Symbol)
			ps.log.Info("position closed", "symbol", pos.Symbol)
			return
		}

		// Добавление/обновление позиции
		if pos.Size != 0 && pos.Side != "" {
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

				if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
					ps.tickerStreamErrorCritical <- err
				}
			}
		}
	}

	errHandler := func(err error, critical bool) {
		if !critical {
			ps.log.Error("position stream error", "error", err)
		} else {
			ps.positionStreamErrorCritical <- err
			ps.hub.UpdateConnection(serviceName, hub.Disconnected, err)
			connectionStatus = hub.Disconnected
		}
	}

	unsubscribe, err := ps.exchange.SubscribePositionStart(ctx, dataHandler, errHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe position stream: %w", err)
	}

	ps.wg.Add(1)
	wrappedUnsubscribe := func() error {
		defer ps.wg.Done()

		ps.hub.RemoveConnection(serviceName)
		if err := unsubscribe(); err != nil {
			ps.log.Error("failed to unsubscribe position")
			return err
		}

		ps.log.Info("unsubscribed position successfully")
		return nil
	}

	return wrappedUnsubscribe, nil

}

func (ps *PositionService) SubscribeTickerStart(ctx context.Context, symbol string) (func() error, error) {

	serviceName := fmt.Sprintf("SubscribeTicker_%s", symbol)
	ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
	var connectionStatus hub.ConnectionStatus = hub.Connected

	dataHandler := func(price float64) {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		if connectionStatus == hub.Disconnected {
			ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
			connectionStatus = hub.Connected
		}

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

	errHandler := func(err error, critical bool) {
		if !critical {
			ps.log.Error("ticker stream error", "symbol", symbol, "error", err)
			ps.hub.UpdateConnection(serviceName, hub.Disconnected, err)
			connectionStatus = hub.Disconnected
		} else {
			ps.tickerStreamErrorCritical <- err
		}
	}

	unsubscribe, err := ps.exchange.SubscribeTickerStart(ctx, symbol, dataHandler, errHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe ticker %s: %w", symbol, err)
	}

	ps.wg.Add(1)
	wrappedUnsubscribe := func() error {
		defer ps.wg.Done()

		ps.hub.RemoveConnection(serviceName)
		if err := unsubscribe(); err != nil {
			ps.log.Error("failed to unsubscribe from ticker", "symbol", symbol, "error", err)
			return err
		}

		ps.log.Info("unsubscribed ticker successfully", "symbol", symbol)
		return nil
	}

	return wrappedUnsubscribe, nil
}

func (ps *PositionService) StopAllSubscriptions() error {

	for symbol, unsub := range ps.subscribedTickers {
		if err := unsub(); err != nil {
			ps.log.Error("failed to unsubscribe ticker", "symbol", symbol, "error", err)
		}
	}

	if ps.subscribedPosition != nil {
		if err := ps.subscribedPosition(); err != nil {
			ps.log.Error("failed to unsubscribe position")
		}
	}

	ps.wg.Wait()
	ps.log.Info("all subscriptions stopped")
	return nil
}
