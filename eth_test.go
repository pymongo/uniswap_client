package main

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// can't define new method on non local type
// func (self *big.Int) FnName() {}
func TestHelloName(t *testing.T) {
	result := "0x000000000000000000000000000000000000000000000000000000135d10239b00000000000000000000000000000000000000000000049e145dd82cd75b9d5500000000000000000000000000000000000000000000000000000000669e4462"
	data, err := os.ReadFile("example.txt")
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
