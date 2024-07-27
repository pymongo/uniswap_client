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
	log.Println("before New")
	u := exchange.NewUniBroker(&conf, ch)
	log.Println("New ok")
	u.Mainloop()
	for {
		m := <- ch
		log.Println(m)
	}
}
