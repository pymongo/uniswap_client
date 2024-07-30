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
	err := u.BuyEth(0, 0.001)
	if err != nil {
		log.Fatalln(err)
	}
	for {
		m := <- ch
		_ = m
		// log.Println(m)
	}
}
