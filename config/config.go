package config

import (
	"log"
	"os"
	"arbitrage/utils"
	"github.com/BurntSushi/toml"
)

type Config struct {
	Key string
	Secret string
	PrivateKey string
}

func NewConfig() Config {
	tomlStr, err := os.ReadFile("config.toml")
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
