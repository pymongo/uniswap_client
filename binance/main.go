package main

import (
	"log"
	"strings"
	"github.com/gorilla/websocket"
)

var symbols = []string{
	"ftmusdc",
}
const (
	BnWsUrl = "wss://stream.binance.com:9443/stream"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	var builder strings.Builder
	builder.Grow(64)
	builder.WriteString(BnWsUrl)
	builder.WriteString("?streams=")
	for i, symbol := range symbols {
		if i != 0 {
			builder.WriteByte('/')
		}
		builder.WriteString(symbol)
		builder.WriteString("@bookTicker")
		// builder.WriteString("%40bookTicker")
		builder.WriteByte('/')
		builder.WriteString(symbol)
		builder.WriteString("@depth5@100ms")
		// builder.WriteString("%40depth5%40100ms")
		// if i < len(symbols)-1 { builder.WriteByte('&') }
	}
	wsUrl := builder.String()
	log.Println("wsUrl", wsUrl)

	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true
	// Error connecting to WebSocket server:websocket: duplicate header not allowed: Sec-Websocket-Extensions
	// header := http.Header{}
	// header.Add("Sec-WebSocket-Extensions", "permessage-deflate")
	conn, _, err := dialer.Dial(wsUrl, nil)
	if err != nil {
		log.Fatal("Error connecting to WebSocket server:", err)
	}
	defer conn.Close()
	for {
		opcode, msg, err := conn.ReadMessage()
		if err != nil {
			log.Fatal("Read error:", err)
			return
		}
		log.Println("opcode", opcode, "msg", string(msg))
	}	
}
