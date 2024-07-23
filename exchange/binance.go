package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
)

//lint:ignore U1000 unused
type BnBroker struct {
	key                string
	secret             []byte
	listenKey          string
	listenKeyCreatedAt int64

	rest http.Client
}

func (bn *BnBroker) req(method, path string, params, headers map[string]string, auth bool, respVal interface{}) error {
	var qs strings.Builder // query_string
	if params != nil {
		qs.Grow(64)
		for key, value := range params {
			qs.WriteString(key)
			qs.WriteByte('&')
			qs.WriteString(value)
		}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	var queryString string
	if auth {
		timeMs := time.Now().UnixMilli()
		timeMsStr := strconv.FormatInt(timeMs, 10)
		headers[BnHeaderKey] = bn.key
		if qs.Len() == 0 {
			qs.WriteByte('&')
		}
		qs.WriteString("timestamp")
		qs.WriteString(timeMsStr)
		queryString = qs.String()
		mac := hmac.New(sha256.New, bn.secret)
		mac.Write([]byte(queryString))
		signature := hex.EncodeToString(mac.Sum(nil))
		queryString += "&signature=" + signature
	} else {
		queryString = qs.String()
	}
	var url string
	if len(queryString) == 0 {
		url = BnRestUrl + path
	} else {
		url = BnRestUrl + path + "?" + queryString
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Fatalln(url, err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := bn.rest.Do(req)
	if err != nil {
		log.Println(url, err)
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(url, err)
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(url + " " + string(respBody))
	}
	err = sonic.Unmarshal(respBody, respVal)
	if err != nil {
		log.Println(url, string(respBody), err)
		return err
	}

	return nil
}

type createListenKeyResp struct {
	ListenKey string `json:"listenKey"`
}

func (bn *BnBroker) createListenKey() error {
	var resp createListenKeyResp
	err := bn.req("POST", "userDataStream", nil, map[string]string{"": ""}, false, &resp)
	if err != nil {
		return err
	}
	bn.listenKey = resp.ListenKey
	bn.listenKeyCreatedAt = time.Now().Unix()
	return err
}

type Empty struct{}

func (bn *BnBroker) renewListenKey() error {
	var resp Empty
	err := bn.req("POST", "userDataStream", nil, map[string]string{"": ""}, false, &resp)
	if err != nil {
		return err
	}
	bn.listenKeyCreatedAt = time.Now().Unix()
	return err
}

const (
	BnRestUrl   = "https://api.binance.com/api/v3/"
	BnWsUrl     = "wss://stream.binance.com:9443/stream"
	BnHeaderKey = "X-MBX-APIKEY"
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
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}
type Depth struct {
	Asks [][]F64 `json:"asks"`
	Bsks [][]F64 `json:"bids"`
	// lastUpdateId u32
}
type BookTicker struct {
	E        int64  `json:"omitempty"` // swap only, spot no this field
	S        string `json:"s"`
	BidPrice F64    `json:"b"`
	B        F64
	AskPrice F64 `json:"a"`
	A        F64
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

func NewBnBroker(key, secret string) BnBroker {
	return BnBroker{
		key:                key,
		secret:             []byte(secret),
		listenKey:          "",
		listenKeyCreatedAt: 0,
		rest:               http.Client {
			// Transport: &http.Transport
			Timeout: 5,
		},
	}
}
func (bn *BnBroker) Init(symbols []string) {
	if len(bn.key) == 0 {
		return
	}
	err := bn.createListenKey()
	if err != nil {
		log.Fatalln(err)
	}
	// TODO private ws
	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true
	conn, _, err := dialer.Dial(publicWsUrl(symbols), nil)
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

func publicWsUrl(symbols []string) string {
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
	return wsUrl
}
