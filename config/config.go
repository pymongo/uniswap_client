package config

import (
	"arbitrage/utils"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/ethereum/go-ethereum/common"
)

type Config struct {
	Key string
	Secret string
	PrivateKey string
	UsdcAddr common.Address
	RpcUrl string
	WsUrl string `json:"omitempty"`
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
		log.Fatalln("err")
	}

	config.Key = utils.AesDecrypt(config.Key)
	config.Secret = utils.AesDecrypt(config.Secret)
	config.PrivateKey = utils.AesDecrypt(config.PrivateKey)

	return config
}
