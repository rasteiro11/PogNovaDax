package main

import (
	"context"
	gosocketio "github.com/ambelovsky/gosf-socketio"
	"github.com/ambelovsky/gosf-socketio/transport"
	"github.com/rasteiro11/PogCore/pkg/logger"
	"github.com/shopspring/decimal"
	"log"
	"net/url"
	"sync"
)

type MarketData struct {
	Ask            decimal.Decimal `json:"ask"`
	BaseVolume24h  decimal.Decimal `json:"baseVolume24h"`
	Bid            decimal.Decimal `json:"bid"`
	High24h        decimal.Decimal `json:"high24h"`
	LastPrice      decimal.Decimal `json:"lastPrice"`
	Low24h         decimal.Decimal `json:"low24h"`
	Open24h        decimal.Decimal `json:"open24h"`
	QuoteVolume24h decimal.Decimal `json:"quoteVolume24h"`
	Symbol         string          `json:"symbol"`
	Timestamp      int64           `json:"timestamp"`
}

func main() {
	ctx := context.Background()

	g := NewWebsocket()

	c, err := g.Start(ctx, "USDT")
	if err != nil {
		log.Printf("ERR: on start %+v\n", err)
	}

	for md := range c {
		log.Printf("MARKET DATA GAMER: %+v\n", md)
	}

	for {
	}
}

type Websocket struct {
	url            *url.URL
	once           sync.Once
	client         *gosocketio.Client
	retry          chan struct{}
	done           chan struct{}
	ready          chan struct{}
	subscriberChan chan []string
	priceStream    chan MarketData
}

func NewWebsocket() *Websocket {
	return &Websocket{
		done:        make(chan struct{}),
		priceStream: make(chan MarketData, 1),
		url: &url.URL{
			Scheme: "wss",
			Host:   "api.novadax.com",
			Path:   "/socket.io/?EIO=3&transport=websocket",
		},
	}
}

func (w *Websocket) subscriber(ctx context.Context) {
	select {
	case <-w.done:
		return
	case message := <-w.subscriberChan:
		select {
		case <-ctx.Done():
			return
		case _, ok := <-w.done:
			if !ok {
				return
			}
		default:
		}

		<-w.ready

		logger.Of(ctx).Debugf("Sending message: %s", message)

		if err := w.client.Emit("SUBSCRIBE", message); err != nil {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-w.done:
				if !ok {
					return
				}
			default:
			}

			logger.Of(ctx).Errorf("SUBSCRIBE ERROR: %+v", err)
			w.retry <- struct{}{}
			return
		}

		for _, topic := range message {
			log.Printf("TOPIC: %+v\n", topic)
			err := w.client.On(topic, func(h *gosocketio.Channel, args MarketData) {
				log.Printf("RECEIVED MARKET DATA: %+v\n", args)
				w.priceStream <- args
			})
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-w.done:
					if !ok {
						return
					}
				default:
				}
				logger.Of(ctx).Errorf("REGISTER HANDLER ERROR: %+v", err)
				w.retry <- struct{}{}
				return
			}
		}
	}
}

func (w *Websocket) Start(ctx context.Context, currency string) (<-chan MarketData, error) {
	w.priceStream = make(chan MarketData, 1)

	go func() {
		defer func() {
			if w.client != nil {
				logger.Of(ctx).Debug("closing database connection")
				w.client.Close()
				w.Close(ctx)
			}
		}()

	Retry:
		w.retry = make(chan struct{}, 1)
		w.ready = make(chan struct{}, 1)
		w.subscriberChan = make(chan []string)

		c, err := gosocketio.Dial(
			"wss://api.novadax.com/socket.io/?EIO=3&transport=websocket",
			transport.GetDefaultWebsocketTransport())
		if err != nil {
			log.Print(err)
		}

		logger.Of(ctx).Debug("Successfully connected")

		// go w.sendMessage(ctx)
		go w.subscriber(ctx)

		w.client = c
		close(w.ready)

		go func() { w.subscriberChan <- []string{"MARKET.USDT_BRL.TICKER", "MARKET.USDT_BRL.TRADE"} }()

		<-w.retry
		goto Retry

	}()

	return w.priceStream, nil
}

func (w *Websocket) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger.Of(ctx).Debug("Closing connection")

	w.once.Do(func() {
		close(w.done)
		close(w.retry)
		close(w.priceStream)
	})

	return nil
}
