package config

import (
	"arbitrage/model"
	"arbitrage/utils"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ethereum/go-ethereum/common"
)

type Config struct {
	Key string
	Secret string
	PrivateKey string
	UsdcAddr common.Address
	RouterAddr common.Address
	RpcUrl string
	WsUrl string `json:"omitempty"`
	DepositAddr common.Address
	Pairs []UniPair
}

func NewConfig() Config {
	configPath := "config.toml"
	if len(os.Args) == 2 {
		configPath = os.Args[1]
	}
	tomlStr, err := os.ReadFile(configPath)
	if err != nil {
        log.Fatalf("Error opening file: %v", err)
    }
		
	var config Config
	if _, err := toml.Decode(string(tomlStr), &config); err != nil {
		log.Fatalln(err)
	}

	config.Key = utils.AesDecrypt(config.Key)
	config.Secret = utils.AesDecrypt(config.Secret)
	config.PrivateKey = utils.AesDecrypt(config.PrivateKey)

	return config
}

type UniPair struct {
	Addr common.Address
	Name string // 只是用于日志打印
	Token0Addr   common.Address
	Token1Addr   common.Address
	Reserve0            *big.Int
	Reserve1            *big.Int
	DecimalsMul0        float64 // e.g. 1e18
	DecimalsMul1        float64
	QuoteIsToken1 bool // e.g. USDC/ETH is false
}
func (pair *UniPair) Amount0() float64 {
	reserve := new(big.Int).Set(pair.Reserve0)
	reserve.Div(reserve, big.NewInt((int64)(pair.DecimalsMul0)))
	amount := new(big.Float).SetInt(reserve)
	float, _ := amount.Float64()
	return float
}
func (pair *UniPair) Amount1() float64 {
	reserve := new(big.Int).Set(pair.Reserve1)
	reserve.Div(reserve, big.NewInt((int64)(pair.DecimalsMul1)))
	amount := new(big.Float).SetInt(reserve)
	float, _ := amount.Float64()
	return float
}
func (pair *UniPair) Price() float64 {
	amount0 := pair.Amount0()
	amount1 := pair.Amount1()

	if pair.QuoteIsToken1 {
		return amount1 / amount0
	} else {
		return amount0 / amount1
	}
}
func (pair *UniPair) Bbo() model.Bbo {
	price := pair.Price()
	return model.Bbo{
		Ask:    price,
		Bid:    price,
		TimeMs: time.Now().UnixMilli(),
	}
}
