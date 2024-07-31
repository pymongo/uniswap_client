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
	// err := u.TransferEth(0.02)
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	err := u.SellEth(0, 12)
	if err != nil {
		log.Fatalln(err)
	}
	for {
		m := <- ch
		_ = m
		// log.Println(m)
	}
}
