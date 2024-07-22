package main

import (
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

var symbols = []string{
	"ftmusdc",
}

// can't define new method on non local type
// func (self *big.Int) FnName() {}
// go test -timeout 30s -run ^TestHelloName$ uniswap
// go test -run TestHelloName
func TestBnWs1(t *testing.T) {
	var channels []string
	for _, symbol := range symbols {
		// 简单的字符串连接情况下 +拼接性能更好不需要解析 不需要解析 format 模板, fmt.Sprintf("%s@bookTicker", symbol) 性能更差
		channels = append(channels, symbol + "%s@bookTicker", symbol + "@depth5@100ms")
	}
	params := strings.Join(channels, "&")
	wsUrl := "wss://stream.binance.com:9443/stream" + params
	_ = wsUrl
}

func TestBnWs2(t *testing.T) {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	var builder strings.Builder
	builder.Grow(64)
	builder.WriteString("wss://stream.binance.com:9443/stream?")
	for i, symbol := range symbols {
		if i != 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(symbol)
		builder.WriteString("@bookTicker")
		builder.WriteByte('&')
		builder.WriteString(symbol)
		builder.WriteString("@depth5@100ms")
		// if i < len(symbols)-1 {
		// 	builder.WriteByte('&')
		// }
	}
	wsUrl := builder.String()
	log.Println("wsUrl", wsUrl)

	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true
	header := http.Header{}
	header.Add("Sec-WebSocket-Extensions", "permessage-deflate")
	conn, _, err := dialer.Dial(wsUrl, header)
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
