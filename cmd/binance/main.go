package main

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
)

var symbols = []string{
	"ftmusdc",
	"celousdc",
	"jupusdc",
}

const (
	BnWsUrl = "wss://stream.binance.com:9443/stream"
)

type F64 float64
// 实现 json.Unmarshaler 接口
func (f *F64) UnmarshalJSON(data []byte) error {
	// 解析 JSON 字符串
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// 将字符串转换为 float64
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return errors.New("invalid float value")
	}

	*f = F64(val)
	return nil
}
// StreamData is a struct that closely resembles the Rust struct.
type StreamData struct {
	Stream string `json:"stream"`
	Data   json.RawMessage `json:"data"`
}
type Depth struct {
	Asks [][]F64 `json:"asks"`
	Bsks [][]F64 `json:"bids"`
	// lastUpdateId u32
}
type BookTicker struct {
	E int64 `json:"omitempty"` // swap only, spot no this field
	S string `json:"s"`
	BidPrice F64 `json:"b"`
	B F64
	AskPrice F64 `json:"a"`
	A F64
}
func handlePublicChannel(symbol string, channel string, data []byte) {
	// transmute/String::from_utf8_unchecked
	// *(*string)(unsafe.Pointer(&b))
	switch channel {
	case "bookTicker":
		var bookTicker BookTicker
		err := sonic.Unmarshal(data, &bookTicker)
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("%#v\n", bookTicker)
	case "depth5@100ms":
		var depth Depth
		err := sonic.Unmarshal(data, &depth)
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("%s %v\n", symbol, depth)
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	/*
		(可选) JIT 预热 bn ws json 类型，JIT 汇编代码仅支持 x86_64 架构
		easyjson 类似 serde_json 编译时 codegen 方式生成 json 反序列化代码避免反射的性能开销
		Go 的反射包（reflect）在一定程度上使用了缓存
	*/
	err := sonic.Pretouch(reflect.TypeOf(StreamData{}))
	if err != nil {
		log.Fatalln(err)
	}

	var builder strings.Builder
	builder.Grow(64)
	builder.WriteString(BnWsUrl)
	builder.WriteString("?streams=")
	for i, symbol := range symbols {
		if i != 0 {
			builder.WriteByte('/')
		}
		builder.WriteString(symbol)
		builder.WriteString("@bookTicker") // websocket 库会自动做 url escape
		builder.WriteByte('/')
		builder.WriteString(symbol)
		builder.WriteString("@depth5@100ms")
		// builder.WriteString("%40depth5%40100ms")
		// if i < len(symbols)-1 { builder.WriteByte('&') }
	}
	wsUrl := builder.String()
	log.Println("wsUrl", wsUrl)
	// dialer default would send ping in interval
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
loop:
	for {
		opcode, msg, err := conn.ReadMessage()
		// 每个 branch 默认会 break 除非 fallthrough
		switch opcode {
		case websocket.TextMessage:
			var streamData StreamData
			err = sonic.Unmarshal(msg, &streamData)
			// err = json.Unmarshal(msg, &streamData)
			if err != nil {
				log.Fatalln(err)
			}
			aIdx := -1
			// range string 会得到 rune 类似于 Rust 的 char
			for i, byte := range ([]byte)(streamData.Stream) {
				if byte == '@' {
					aIdx = i
					break
				}
			}
			if aIdx == -1 {
				log.Println("TODO handle private channel", string(streamData.Data))
			} else {
				// transmute/String::from_utf8_unchecked
				// *(*string)(unsafe.Pointer(&b))
				symbol := streamData.Stream[:aIdx]
				channel := streamData.Stream[aIdx+1:]
				handlePublicChannel(symbol, channel, streamData.Data)
			}
		case websocket.BinaryMessage:
			log.Println(string(msg))
		case websocket.PingMessage:
			err = conn.WriteMessage(websocket.PongMessage, msg)
			if err != nil {
				log.Fatalln(err)
			}
		case websocket.PongMessage:
			log.Println("recv pong", string(msg))
		case websocket.CloseMessage:
			log.Fatalln("server send close")
		default:
			log.Println("unhandle opcode", opcode, string(msg))
		}
		if err != nil {
			log.Fatal("Read error:", err)
			break loop
		}
	}
}
