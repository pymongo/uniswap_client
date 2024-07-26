package main

import (
	"arbitrage/config"
	"arbitrage/exchange"
	"arbitrage/model"
	"log"

	"time"
)

type StrategyState struct {
}

type ExchangeState struct {
}

type HedgePair struct {
}

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	conf := config.NewConfig()
	leadBboCh := make(chan model.Bbo, 128)
	lagBboCh := make(chan model.Bbo, 128)
	lead := exchange.NewUniBroker(conf.PrivateKey, leadBboCh)
	lag := exchange.NewBnBroker(conf.Key, conf.Secret, lagBboCh)
	go lead.Mainloop()
	go lag.Mainloop([]string{"ftmusdt"})
	leadPrice := 0.
	lagPrice := 0.
	for {
		select {
		case leadBbo := <-leadBboCh:
			leadPrice = leadBbo.Ask
			calcPriceSpread(leadPrice, lagPrice)
		case lagBbo := <-lagBboCh:
			lagPrice = lagBbo.Ask
			calcPriceSpread(leadPrice, lagPrice)
		}
	}
}

func calcPriceSpread(leadPrice float64, lagPrice float64) {
	now := time.Now().UnixNano()
	// use time % to reduce log frequency
	if now%4 == 0 {
		log.Println("lead Uniswap", leadPrice, "lag BnUsdcSpot", lagPrice)
	}
	spread := (lagPrice - leadPrice) / leadPrice
	if spread <= 0.0005 {
		return
	}
	log.Println("Price discovery: should buy in lead exchange and sell in lag exchange spread =", spread)
}
