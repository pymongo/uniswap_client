package exchange

import (
	"arbitrage/model"
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

const (
	BnRestUrl   = "https://api.binance.com"
	BnWsUrl     = "wss://stream.binance.com:9443/stream"
	BnHeaderKey = "X-MBX-APIKEY"
)

//lint:ignore U1000 unused
type BnBroker struct {
	key                string
	secret             []byte
	listenKey          string
	listenKeyCreatedAt int64
	bboCh              chan model.Bbo
	rest               http.Client
	interestRate       float64
	Assets             map[string]asset
}

func (bn *BnBroker) req(method, path string, params, headers map[string]string, auth bool, respVal interface{}) error {
	var qs strings.Builder // query_string
	if params != nil {
		qs.Grow(64)
		i := 0
		for key, value := range params {
			if i > 0 {
				qs.WriteByte('&')
			}
			qs.WriteString(key)
			qs.WriteByte('=')
			qs.WriteString(value)
			i += 1
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
		if qs.Len() > 0 {
			qs.WriteByte('&')
		}
		qs.WriteString("timestamp=")
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
	err := bn.req("POST", "/api/v3/userDataStream", nil, map[string]string{BnHeaderKey: bn.key}, false, &resp)
	if err != nil {
		return err
	}
	bn.listenKey = resp.ListenKey
	bn.listenKeyCreatedAt = time.Now().Unix()
	return err
}

func (bn *BnBroker) renewListenKey() error {
	var resp json.RawMessage
	err := bn.req("PUT", "/api/v3/userDataStream", map[string]string{"listenKey": bn.listenKey}, map[string]string{BnHeaderKey: bn.key}, false, &resp)
	if err != nil {
		return err
	}
	bn.listenKeyCreatedAt = time.Now().Unix()
	return err
}

// 还有个现货杠杆下单api是 oco 后缀的 ​OCO (One-Cancels-the-Other) 止盈止损单被触发，或限价订单成交或部分成交时，另一个订单则自动撤销
// type PostMarginOrder struct {
// 	Symbol string `json:"symbol"`
// 	Side string `json:"side"`
// 	// 不需要定义请求结构体，反正生成签名和传输也是用 map[string]string
// }

// FTM借币10开空GTC，没成交撤单后，负债是一小时利息0.00003257
// FTM借币10开空IOC，没成交撤单后，负债是10FTM+一小时利息(0.00006514-0.00003257)
func (bn *BnBroker) PostMarginOrder(p model.PostOrderParams) error {
	params := map[string]string {
		"symbol": p.Symbol,
		"quantity": strconv.FormatFloat(p.Amount, 'f', -1, 64),
		"sideEffectType": "AUTO_BORROW_REPAY",
	}
	if p.Side == model.SideBuy {
		params["side"] = "BUY"
	} else {
		params["side"] = "SELL"
	}
	if p.Tif != model.TifMarket {
		params["type"] = "LIMIT"
		params["price"] = strconv.FormatFloat(p.Price, 'f', -1, 64)
		if p.Tif == model.TifGtc {
			params["timeInForce"] = "GTC"
		} else {
			params["timeInForce"] = "IOC"
		}
	} else {
		params["type"] = "MARKET"
		params["timeInForce"] = "IOC"
	}
	// IOC 没成交返回 {"symbol":"FTMUSDT","orderId":3432590431,"clientOrderId":"U3vAoKlfhxWPDENJLH58vI","transactTime":1721963358518,"price":"0.5","origQty":"10","executedQty":"0","cummulativeQuoteQty":"0","status":"EXPIRED","timeInForce":"IOC","type":"LIMIT","side":"SELL","fills":[],"marginBuyBorrowAsset":"FTM","marginBuyBorrowAmount":"10","isIsolated":false,"selfTradePreventionMode":"EXPIRE_MAKER"}
	var r json.RawMessage
	err := bn.req("POST", "/sapi/v1/margin/order", params, map[string]string{}, true, &r)
	if err != nil {
		return err
	}
	log.Println(string(r))
	return err
}

// 还币/借币 没有欠款时调用还钱不会报错，也无事发生
func (bn *BnBroker) Repay(asset string, amount float64) error {
	type_ := "REPAY"
	if amount < 0 {
		type_ = "BORROW"
		amount = -amount
	}
	params := map[string]string {
		"type": type_,
		"asset": asset,
		"amount": strconv.FormatFloat(amount, 'f', -1, 64),
	}
	var r json.RawMessage
	err := bn.req("POST", "/sapi/v1/margin/borrow-repay", params, map[string]string{}, true, &r)
	log.Println(string(r))
	return err
}

type Interest struct {
	Asset string
	NextHourlyInterestRate F64
}

func (bn *BnBroker) getInterestRate() error {
	params := map[string]string {
		"assets": "USDT,USDC,FDUSD,BNB,FTM,CELO",
		"isIsolated": "FALSE",
	}
	var r []Interest
	err := bn.req("GET", "/sapi/v1/margin/next-hourly-interest-rate", params, map[string]string{}, true, &r)
	if err != nil {
		return err
	}
	for i := range r {
		if r[i].Asset == "FTM" {

			break;
		}
	}
	return err
}

func (bn *BnBroker) getPosition() error {
	var r marginPos
	err := bn.req("GET", "/sapi/v1/margin/account", map[string]string{}, map[string]string{}, true, &r)
	if err != nil {
		return err
	}
	for i := range r.UserAssets {
		coin := r.UserAssets[i].Asset
		if coin == "USDC" || coin == "USDT" || coin == "BNB" || coin == "FTM" {
			bn.Assets[coin] = r.UserAssets[i]
		}
	}
	log.Printf("币安资产 %f$, %#v", r.TotalCollateralValueInUSDT, bn.Assets)
	return err
}

type marginPos struct {
	TotalCollateralValueInUSDT F64 `json:"TotalCollateralValueInUSDT"`
	UserAssets []asset `json:"userAssets"`
}
type asset struct {
	Asset    string `json:"asset,omitempty"`
	Free     F64    `json:"free,omitempty"`
	Locked   F64    `json:"locked,omitempty"`
	Borrow   F64    `json:"borrow,omitempty"`
	Interest F64    `json:"interest,omitempty"`
	NetAsset F64    `json:"NetAsset,omitempty"`
}

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

func (b BookTicker) bbo() model.Bbo {
	return model.Bbo{
		Ask:    float64(b.AskPrice),
		Bid:    float64(b.BidPrice),
		TimeMs: time.Now().UnixMilli(),
	}
}

func handlePublicChannel(symbol string, channel string, data []byte, bboCh chan model.Bbo) {
	// transmute/String::from_utf8_unchecked
	// unsafe.String(unsafe.SliceData(b.buf), len(b.buf)), *(*string)(unsafe.Pointer(&b))
	switch channel {
	case "bookTicker":
		var bookTicker BookTicker
		err := sonic.Unmarshal(data, &bookTicker)
		if err != nil {
			log.Fatalln(err)
		}
		// log.Printf("%#v\n", bookTicker)
		bboCh <- bookTicker.bbo()
	case "depth5@100ms":
		var depth Depth
		err := sonic.Unmarshal(data, &depth)
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("%s %v\n", symbol, depth)
	}
}

func NewBnBroker(key, secret string, bboCh chan model.Bbo) BnBroker {
	return BnBroker{
		key:                key,
		secret:             []byte(secret),
		listenKey:          "",
		listenKeyCreatedAt: 0,
		rest: http.Client{
			// Transport: &http.Transport
			Timeout: 5 * time.Second,
		},
		Assets: map[string]asset{},
		bboCh: bboCh,
	}
}
func (bn *BnBroker) Mainloop(symbols []string) {
	if len(bn.key) != 0 {
		err := bn.createListenKey()
		if err != nil {
			log.Fatalln(err)
		}
		// TODO private ws
	}
	err := bn.getPosition()
	if err != nil {
		log.Fatalln(err)
	}
	// err = bn.getInterestRate()
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	// bn.Repay("FTM", 1)
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	go func() {
		err = bn.publicWsMainLoop(symbols)
		if err != nil {
			log.Println("bn.publicWsMainLoop", err)
		}
		time.Sleep(1 * time.Second)
	}()
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
		// builder.WriteByte('/')
		// builder.WriteString(symbol)
		// builder.WriteString("@depth5@100ms")
	}
	wsUrl := builder.String()
	return wsUrl
}

func (bn *BnBroker) publicWsMainLoop(symbols []string) error {
	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true
	conn, _, err := dialer.Dial(publicWsUrl(symbols), nil)
	if err != nil {
		log.Fatal("Error connecting to WebSocket server:", err)
	}
	defer conn.Close()

	for {
		opcode, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		// 每个 branch 默认会 break 除非 fallthrough
		switch opcode {
		case websocket.TextMessage:
			var streamData StreamData
			err = sonic.Unmarshal(msg, &streamData)
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
				symbol := streamData.Stream[:aIdx]
				channel := streamData.Stream[aIdx+1:]
				handlePublicChannel(symbol, channel, streamData.Data, bn.bboCh)
			}
		case websocket.BinaryMessage:
			log.Println(string(msg))
		case websocket.PingMessage:
			err = conn.WriteMessage(websocket.PongMessage, msg)
			if err != nil {
				return err
			}
		case websocket.PongMessage:
			log.Println("recv pong", string(msg))
		case websocket.CloseMessage:
			log.Fatalln("server send close")
		default:
			log.Println("unhandle opcode", opcode, string(msg))
		}
	}
}

func (bn *BnBroker) lastPrice(symbol string) {
	// if no symbol params prices for all symbols will be returned in an array
	// bn.req("GET", "/api/v3/ticker/price")
}
