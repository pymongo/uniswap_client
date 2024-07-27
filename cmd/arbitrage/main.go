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
	// log.Printf("%#v", conf)
	uniBboCh := make(chan model.Bbo, 128)
	bnBboCh := make(chan model.Bbo, 128)
	uni := exchange.NewUniBroker(&conf, uniBboCh)
	bn := exchange.NewBnBroker(conf.Key, conf.Secret, bnBboCh)
	uni.Mainloop()
	bn.Mainloop([]string{"ftmusdt"})
	
	// log.Printf("%#v\n", bn.Assets)

	uniPrice := 0.
	bnPrice := 0.
	for {
		select {
		case uniBbo := <-uniBboCh:
			uniPrice = uniBbo.Bid
			if bnPrice == 0 {
				continue
			}
			calcPriceSpread(uniPrice, bnPrice)
		case bnBbo := <-bnBboCh:
			bnPrice = bnBbo.Ask
			if uniPrice == 0 {
				continue
			}
			calcPriceSpread(uniPrice, bnPrice)
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
