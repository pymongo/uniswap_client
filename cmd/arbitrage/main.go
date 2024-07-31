package main

import (
	"arbitrage/config"
	"arbitrage/exchange"
	"arbitrage/model"
	"log"
	"sync"

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
			onTick(uniPrice, bnPrice, &uni, &bn)
		case bnBbo := <-bnBboCh:
			bnPrice = bnBbo.Ask
			if uniPrice == 0 {
				continue
			}
			onTick(uniPrice, bnPrice, &uni, &bn)
		}
	}
}

func onTick(uniPrice float64, bnPrice float64, uni *exchange.UniBroker, bn *exchange.BnBroker) {
	spread := (bnPrice - uniPrice) / uniPrice
	now := time.Now().UnixNano()
	if now % 5 == 0 {
		log.Println("lead Uniswap", uniPrice, "lag Bn", bnPrice, "spread", spread)
	}	
	// if spread <= 0.0015 {
	// 	return
	// }
	log.Println("链上价格", uniPrice, "币安价格", bnPrice, "价差", spread)
	if uni.Usdc < 6 { // || bn.Assets 
		log.Fatalln("Uniswap not enough assets", uni.Usdc)
	}
	uniBeforeUsdc := uni.Usdc
	uniBeforeEth := uni.Eth
	amount := 12.0
	var wg sync.WaitGroup
	go func() {
		defer wg.Done()
		err := uni.BuyEth(0, amount)
		if err != nil {
			log.Fatalln(err)
		}
	}()	
	go func() {
		defer wg.Done()
		err := bn.PostMarginOrder(model.PostOrderParams{
			Symbol: "FTMUSDT",
			Side:   model.SideSell,
			Amount: amount,
			Tif:    model.TifMarket,
		})
		if err != nil {
			log.Fatalln(err)
		}
	}()
	wg.Wait()
	log.Println("对冲下单完成!")
	log.Println("等下次链上轮询余额")
	time.Sleep(5 * time.Second)
	log.Println("链上资产变化 ETH,USDC",uniBeforeEth,uniBeforeUsdc,"->",uni.Eth,uni.Usdc)
	err := uni.TransferEth(amount)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("等提币到币安到账")
	time.Sleep(145 * time.Second)
	log.Println("币安准备还债")
	err = bn.Repay("FTM", amount)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("币安还债成功")
}
