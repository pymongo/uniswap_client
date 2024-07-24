package main

import (
	"log"
	"uniswap/config"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	conf := config.NewConfig()
	_ = conf
	// ch := make(chan model.Bbo, 128)
	// bn := exchange.NewBnBroker(conf.Key, conf.Secret, ch)
	// bn.Init([]string{"ftmusdc"})
}
