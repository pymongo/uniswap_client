```go
	for i, elem := range batch {
		pair := u.conf.Pairs[i]
		pairAddress := pair.Addr
		if elem.Error != nil {
			log.Fatalf("Error fetching reserves for pair %s: %v", pairAddress, elem.Error)
			continue
		}
		reserveData := (*elem.Result.(*hexutil.Bytes))
		values, err := GetReserves.Outputs.UnpackValues(reserveData)
		if err != nil {
			log.Fatalln(err)
		}
		var reserve GetReservesOutput
		err = GetReserves.Outputs.Copy(&reserve, values)
		if err != nil {
			log.Fatalln(err)
		}
		if reserve.Reserve0 == nil {
			log.Fatalln("reserve.Reserve0 == nil")
		}
		pair.Reserve0 = reserve.Reserve0
		pair.Reserve1 = reserve.Reserve1
		log.Printf("%#v", pair)
		u.bboCh <- pair.Bbo()
	}
	log.Printf("%#v",  u.conf.Pairs[0])
```

16:19:25.499272 uniswap.go:230: config.UniPair{Addr:0x084F933B6401a72291246B5B5eD46218a68773e6, Name:"axlUSDC/WFTM", Token0Addr:0x1B6382DBDEa11d97f24495C9A90b7c88469134a4, Token1Addr:0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83, Reserve0:1319928064066, Reserve1:2901178598542032396285977, DecimalsMul0:1e+18, DecimalsMul1:1e+06, QuoteIsToken1:false}
16:19:25.499526 uniswap.go:233: config.UniPair{Addr:0x084F933B6401a72291246B5B5eD46218a68773e6, Name:"axlUSDC/WFTM", Token0Addr:0x1B6382DBDEa11d97f24495C9A90b7c88469134a4, Token1Addr:0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83, Reserve0:<nil>, Reserve1:<nil>, DecimalsMul0:1e+18, DecimalsMul1:1e+06, QuoteIsToken1:false}

解决办法:

>		pair := u.conf.Pairs[i]

改成

> 		pair := &u.conf.Pairs[i]

避免 pair 发生克隆导致无法修改原数据