#!/bin/bash
set -exu
bin=latency
target=""
r=${r:-lb1}

go build binance/main.go
proxychains rsync -avz --progress main $r:main
proxychains ssh $r /root/main
