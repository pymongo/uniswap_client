package main

import (
	"arbitrage/config"
	"arbitrage/exchange"
	"arbitrage/model"
	"log"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	conf := config.NewConfig()
	_ = conf
	ch := make(chan model.Bbo, 128)
	bn := exchange.NewBnBroker(conf.Key, conf.Secret, ch)
	// bn.Mainloop([]string{"ftmusdc"})
	err := bn.PostMarginOrder(model.PostOrderParams{
		Symbol: "FTMUSDC",
		Side:   model.SideSell,
		Price:  0.5,
		Amount: 10,
	})
	if err != nil {
		log.Fatalln(err)
	}
}
