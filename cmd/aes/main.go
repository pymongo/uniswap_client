package main

import (
	"arbitrage/utils"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)
	// encryptFlag := flag.Bool("e", false, "Encrypt the message")
	args := os.Args[1:]
	if len(args) != 2 {
		log.Fatalln("usage ./aes -e msg or ./aes -d msg")
	}
	if args[0] == "-e" {
		msg := utils.AesEncrypt(args[1])
		log.Println(msg)
	} else {
		msg := utils.AesDecrypt(args[1])
		log.Println(msg)
	}
}
