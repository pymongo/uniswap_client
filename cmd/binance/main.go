package main

import (
	"log"
	"uniswap/config"
	"uniswap/exchange"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	conf := config.NewConfig()
	bn := exchange.NewBnBroker(conf.Key, conf.Secret)
	bn.Init([]string{"ftmusdc"})
}
