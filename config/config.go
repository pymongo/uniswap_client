package config

import (
	"log"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Key string
	Secret string
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

	return config
}
