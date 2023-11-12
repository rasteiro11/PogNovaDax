package marketdata

import (
	"context"
	"pognovadax/models"
)

type MarketDataProvider interface {
	Start(ctx context.Context, currency ...string) (<-chan models.MarketData, error)
	Close(ctx context.Context) error
}
