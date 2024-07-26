package model

type PostOrderParams struct {
	// Cid string
	Symbol string
	Side Side
	Price float64
	Amount float64
	Tif Tif
	// PostOnly bool
}

type Side uint8
const (
    SideBuy Side = iota
    SideSell
)
type Tif uint8
const (
	TifIoc Tif = iota
	TifGtc
	TifMarket
)
