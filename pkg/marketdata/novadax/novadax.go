package novadax

import (
	"context"
	"fmt"
	gosocketio "github.com/ambelovsky/gosf-socketio"
	"github.com/ambelovsky/gosf-socketio/transport"
	"github.com/rasteiro11/PogCore/pkg/logger"
	"log"
	"net/url"
	"pognovadax/models"
	"pognovadax/pkg/marketdata"
	"sync"
)

type websocket struct {
	url            *url.URL
	once           sync.Once
	client         *gosocketio.Client
	retry          chan struct{}
	done           chan struct{}
	ready          chan struct{}
	subscriberChan chan []string
	priceStream    chan models.MarketData
}

func NewMarketDataProvider() marketdata.MarketDataProvider {
	return &websocket{
		done:        make(chan struct{}),
		priceStream: make(chan models.MarketData, 1),
		url: &url.URL{
			Scheme: "wss",
			Host:   "api.novadax.com",
			Path:   "/socket.io/?EIO=3&transport=websocket",
		},
	}
}

func (w *websocket) subscriber(ctx context.Context) {
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
			err := w.client.On(topic, func(h *gosocketio.Channel, args models.MarketData) {
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

func prepareSubscriberPayload(currency ...string) []string {
	payload := []string{}

	for _, c := range currency {
		payload = append(payload, fmt.Sprintf("MARKET.%s_BRL.TICKER", c))
	}

	return payload
}

func (w *websocket) Start(ctx context.Context, currency ...string) (<-chan models.MarketData, error) {
	w.priceStream = make(chan models.MarketData, 1)

	subscriberPayload := prepareSubscriberPayload(currency...)

	go func() {
		defer func() {
			if w.client != nil {
				logger.Of(ctx).Debug("closing database connection")
				w.client.Close()
				if err := w.Close(ctx); err != nil {
					return
				}
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

		go w.subscriber(ctx)

		w.client = c
		close(w.ready)

		go func() { w.subscriberChan <- subscriberPayload }()

		<-w.retry
		goto Retry

	}()

	return w.priceStream, nil
}

func (w *websocket) Close(ctx context.Context) error {
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
