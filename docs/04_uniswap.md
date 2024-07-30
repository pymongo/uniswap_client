## 卖出ETH的tx数据讲解
https://ftmscan.com/tx/0xc53bd52c5b8ad485a8e2af7e4fcbf33151103e368a0817b084426daae0acbafb

swapExactETHForTokens inputs:

1. amountOutMin: 最少得到多少USDC, 价格 * ETH数量 * (1-fee-slipage) 如果没被MEV实际换到的应该比这个数更多
2. path: 0 是支付的币 1 是得到的币
3. to: swap得到的币打给谁，填自己地址
4. deadline: expired in unix timestamp

如果有 ETH 没有 WETH 想要出售 WETH 交易的 value 字段需要写入等量的 ETH 数量

合约会去 WETH 合约调用 deposit(ETH->WETH), deposit/withdraw 不是 ERC20 标准的方法只是 WETH 合约特有的

```
写了一周 DEX/CEX 之间量化交易价差套利，总算把 UniswapV2 交易过程理清楚了

滑点计算(防止被 MEV 夹子攻击)和Price Impact计算比 CEX 下单复杂多了

所有报错都execution revert没有日志难debug

例如有个细节是买ETH时不传value字段，如果有ETH没有WETH想要出售的value字段需要写入等量的ETH数量
```
