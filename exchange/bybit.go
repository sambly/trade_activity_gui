package exchange

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strconv"
	"time"

	"github.com/hirokisan/bybit/v2"
	"github.com/jpillora/backoff"
)

const maxReconnectAttempts = 10

func createLogAdapter(slogLogger *slog.Logger) *log.Logger {
	slogLogger = slogLogger.With("component", "bybit_lib")

	reader, writer := io.Pipe()

	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			slogLogger.Info(scanner.Text())
		}
	}()

	return log.New(writer, "", 0)
}

type Bybit struct {
	log      *slog.Logger
	client   *bybit.Client
	wsClient *bybit.WebSocketClient
}

func NewBybit(key, secret string, logger *slog.Logger, debug bool) *Bybit {

	client := bybit.NewClient().WithAuth(key, secret).WithLogger(createLogAdapter(logger)).WithDebug(debug)
	wsClient := bybit.NewWebsocketClient().WithAuth(key, secret).WithLogger(createLogAdapter(logger)).WithDebug(debug)

	return &Bybit{
		log:      logger.With("component", "bybit"),
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
		return nil, fmt.Errorf("get position info: %w", err)
	}

	positions := make([]Position, 0, len(resp.Result.List))

	for _, posEx := range resp.Result.List {
		pos := Position{
			Symbol:      string(posEx.Symbol),
			Side:        string(posEx.Side),
			CreatedTime: posEx.CreatedTime,
		}

		pos.Size, _ = strconv.ParseFloat(posEx.Size, 64)
		pos.EntryPrice, _ = strconv.ParseFloat(posEx.AvgPrice, 64)
		pos.UnrealisedPnl, _ = strconv.ParseFloat(posEx.UnrealisedPnl, 64)

		positions = append(positions, pos)
	}

	return positions, nil
}

func (b *Bybit) SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error)) error {

	reconnectAttempt := 0
	ba := &backoff.Backoff{
		Min:    30 * time.Second,
		Max:    10 * time.Minute,
		Factor: 2,
		Jitter: true,
	}

	svc, err := b.wsClient.V5().Private()
	if err != nil {
		return fmt.Errorf("create private ws client: %w", err)
	}

	if err := svc.Subscribe(); err != nil {
		return fmt.Errorf("subscribe ws: %w", err)
	}

	unsubscribe, err := svc.SubscribePosition(func(msg bybit.V5WebsocketPrivatePositionResponse) error {

		for _, posEx := range msg.Data {
			pos := Position{
				Symbol:      string(posEx.Symbol),
				Side:        string(posEx.Side),
				CreatedTime: posEx.CreatedTime,
			}

			pos.Size, _ = strconv.ParseFloat(posEx.Size, 64)
			pos.EntryPrice, _ = strconv.ParseFloat(posEx.EntryPrice, 64)
			pos.UnrealisedPnl, _ = strconv.ParseFloat(posEx.UnrealisedPnl, 64)
			pos.CumRealisedPnl, _ = strconv.ParseFloat(posEx.CumRealisedPnl, 64)

			onData(pos)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("subscribe position: %w", err)
	}

	errHandler := func(isWebsocketClosed bool, err error) {

		if onError != nil {
			onError(fmt.Errorf("websocket position error (closed=%t): %w", isWebsocketClosed, err))
		}
	}

	go func() {
		for {
			startDone := make(chan error, 1)

			go func() {
				timerCtx, cancelTimer := context.WithCancel(ctx)
				defer cancelTimer()

				// Таймер успешного подключения для обнуления счетчика попыток
				go func() {
					select {
					case <-time.After(60 * time.Second):
						reconnectAttempt = 0
						ba.Reset()
						b.log.Info("websocket position connection stable, reset reconnect attempts")
					case <-timerCtx.Done():
						return
					}
				}()

				startDone <- svc.Start(ctx, errHandler)
				cancelTimer()
			}()

			select {
			case <-ctx.Done():
				return
			case err := <-startDone:
				if err != nil {

					if onError != nil {
						onError(fmt.Errorf("websocket position start error: %w", err))
					}
				}

				select {
				case <-ctx.Done():
					return
				default:
					reconnectAttempt++
					if reconnectAttempt >= maxReconnectAttempts {

						if onError != nil {
							onError(fmt.Errorf("websocket position max reconnection attempts reached (%d)", maxReconnectAttempts))
						}
						return
					}

					delay := ba.Duration()
					b.log.Info("reconnecting websocket",
						"attempt", reconnectAttempt,
						"delay", delay)

					time.Sleep(delay)
					continue
				}
			}
		}
	}()

	b.log.Info("position subscription started successfully")
	_ = unsubscribe
	return nil
}

func (b *Bybit) SubscribeTickerStart(ctx context.Context, symbol string, onData func(price float64), onError func(err error)) (func() error, error) {
	b.log.Info("starting ticker subscription", "symbol", symbol)

	reconnectAttempt := 0
	ba := &backoff.Backoff{
		Min:    30 * time.Second,
		Max:    10 * time.Minute,
		Factor: 2,
		Jitter: true,
	}

	pubSvc, err := b.wsClient.V5().Public(bybit.CategoryV5Linear)
	if err != nil {
		return nil, fmt.Errorf("create public ws client: %w", err)
	}

	unsubscribe, err := pubSvc.SubscribeTicker(
		bybit.V5WebsocketPublicTickerParamKey{Symbol: bybit.SymbolV5(symbol)},
		func(msg bybit.V5WebsocketPublicTickerResponse) error {

			if msg.Data.LinearInverse.LastPrice != "" {
				price, err := strconv.ParseFloat(msg.Data.LinearInverse.LastPrice, 64)
				if err != nil {
					return nil
				}
				onData(price)
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe ticker %s: %w", symbol, err)
	}

	errHandler := func(isWebsocketClosed bool, err error) {
		if onError != nil {
			onError(fmt.Errorf("websocket ticker error (closed=%t, symbol=%s): %w", isWebsocketClosed, symbol, err))
		}
	}

	go func() {
		for {
			startDone := make(chan error, 1)

			go func() {
				timerCtx, cancelTimer := context.WithCancel(ctx)
				defer cancelTimer()

				// Таймер успешного подключения для обнуления счетчика попыток
				go func() {
					select {
					case <-time.After(60 * time.Second):
						reconnectAttempt = 0
						ba.Reset()
						b.log.Debug("websocket ticker connection stable", "symbol", symbol)
					case <-timerCtx.Done():
						return
					}
				}()

				startDone <- pubSvc.Start(ctx, errHandler)
				cancelTimer()
			}()

			select {
			case <-ctx.Done():
				return
			case err := <-startDone:
				if err != nil {

					if onError != nil {
						onError(fmt.Errorf("websocket ticker start error (symbol=%s): %w", symbol, err))
					}
				}

				select {
				case <-ctx.Done():
					return
				default:
					reconnectAttempt++
					if reconnectAttempt >= maxReconnectAttempts {

						if onError != nil {
							onError(fmt.Errorf("websocket ticker max reconnection attempts reached (%d, symbol=%s)", maxReconnectAttempts, symbol))
						}
						return
					}

					delay := ba.Duration()
					b.log.Info("reconnecting ticker websocket",
						"symbol", symbol,
						"attempt", reconnectAttempt,
						"delay", delay)

					time.Sleep(delay)
					continue
				}
			}
		}
	}()

	b.log.Info("ticker subscription started", "symbol", symbol)

	wrappedUnsubscribe := func() error {
		if err := unsubscribe(); err != nil {
			return fmt.Errorf("unsubscribe ticker %s: %w", symbol, err)
		}
		b.log.Info("ticker unsubscribed", "symbol", symbol)
		return nil
	}

	return wrappedUnsubscribe, nil
}
