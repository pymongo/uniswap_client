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
	ch := make(chan model.Bbo, 128)
	u := exchange.NewUniBroker(&conf, ch)
	u.Mainloop()
	err := u.Swap(exchange.Pairs[exchange.PairAddr], model.SideSell, 0.001)
	// err := u.TransferEth(0.003)
	if err != nil {
		log.Fatalln(err)
	}
	for {
		m := <- ch
		_ = m
		// log.Println(m)
	}
}
