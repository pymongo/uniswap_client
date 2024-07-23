package main

/*
#include <time.h>
time_t time(time_t *tloc);
*/
import "C"
import (
	"log"
	"unsafe"
)

func Time() int64 {
	var n int64
	C.time((*C.time_t)(unsafe.Pointer(&n)))
	return n
}

func main() {
	log.Println(Time())
}
