package main

import (
	"context"
	"log"
	"pognovadax/pkg/marketdata/novadax"
)

func main() {
	ctx := context.Background()

	g := novadax.NewMarketDataProvider()

	c, err := g.Start(ctx, "USDT", "BTC", "ETH")
	if err != nil {
		log.Fatalf("ERR: on start %+v\n", err)
	}

	for md := range c {
		log.Printf("RECEIVED MARKET DATA: %+v\n", md)
	}

	for {
	}
}
