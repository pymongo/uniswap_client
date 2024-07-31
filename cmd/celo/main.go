package main

import (
	"arbitrage/config"
	"arbitrage/exchange/bindings"
	"log"

	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	conf := config.NewConfig()
	client, err := ethclient.Dial(conf.RpcUrl)
	if err != nil {
		log.Fatalln(err)
	}
	router, err := bindings.NewUniswapV2RouterCaller(conf.RouterAddr, client)
	if err != nil {
		log.Fatalln(err)
	}
	factoryAddr, err := router.Factory(nil)
	if err != nil {
		log.Fatalln(err)
	}
	factory, err := bindings.NewUniswapV2FactoryCaller(factoryAddr, client)
	if err != nil {
		log.Fatalln(err)
	}
	numPairs, err := factory.AllPairsLength(nil)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("numPairs", numPairs)
}
