package main

import (
	"arbitrage/config"
	"arbitrage/exchange"
	"log"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	conf := config.NewConfig()
	exchange.NewUniBroker(conf.PrivateKey, nil)	
}
