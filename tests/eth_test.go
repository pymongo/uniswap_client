package tests

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"
	"uniswap/config"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// can't define new method on non local type
// func (self *big.Int) FnName() {}
// go test -timeout 30s -run ^TestHelloName$ uniswap
// go test -run TestHelloName
func TestHelloName(t *testing.T) {
	result := "0x000000000000000000000000000000000000000000000000000000135d10239b00000000000000000000000000000000000000000000049e145dd82cd75b9d5500000000000000000000000000000000000000000000000000000000669e4462"
	data, err := os.ReadFile("IUniswapV2Pair.abi.json")
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	pairAbi, err := abi.JSON(strings.NewReader(string(data)))
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}
	method, exists := pairAbi.Methods["getReserves"]
	if !exists {
		log.Fatal("pairAbi.Methods")
	}
	// marshal
	_ = method.Outputs
	_ = []byte(result[2:])
}

func TestJson(t *testing.T) {
	type S1 struct {
		Stream string
	}
	type S2 struct {
		Stream string `json:"stream"`
	}
	msg := []byte(`{"stream":"foo"}`)
	msg2 := []byte(`{"Stream":"foo"}`)

	var s1 S1
	err := json.Unmarshal(msg, &s1)
	if err != nil {
		log.Fatalln(err)
	}
	if s1.Stream != "foo" {
		log.Fatalln("not ok")
	}
	err = json.Unmarshal(msg2, &s1)
	if err != nil {
		log.Fatalln(err)
	}
	if s1.Stream != "foo" {
		log.Fatalln("not ok")
	}

	var s2 S2
	err = json.Unmarshal(msg2, &s2)
	if err != nil {
		log.Fatalln(err)
	}
	if s2.Stream != "foo" {
		log.Fatalln("not ok")
	}
	err = json.Unmarshal(msg, &s2)
	if err != nil {
		log.Fatalln(err)
	}
	if s2.Stream != "foo" {
		log.Fatalln("not ok")
	}
}

func TestConfig(t *testing.T) {
	pwd, _ := os.Getwd()
	log.Println("pwd", pwd)
	os.Chdir("..")
	pwd, _ = os.Getwd()
	log.Println("pwd", pwd)
	conf := config.NewConfig()
	log.Println(conf.Key)
}
