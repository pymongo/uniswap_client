
## Go 有没有类似 serde_json 编译时 codegen 的库

easyjson 编译时 codegen 避免反射的性能开销

## JIT 预热 sonic

```
err := sonic.Pretouch(reflect.TypeOf(StreamData{})) // 其实也没必要 Go 的反射包（reflect）在一定程度上使用了缓存
(可选) JIT 预热 bn ws json 类型，JIT 汇编代码仅支持 x86_64 架构，
```

## go json 字段不区分大小写

首先 Go reflect 反射机制得到的字段名字一定是区分大小写的，但 json 反序列化可不区分大小写，ETH RLP 反序列根据字段顺序这两都大小写无关

https://pkg.go.dev/encoding/json#Unmarshal

> preferring an exact match but also accepting a case-insensitive match.

```go
func TestJson(t *testing.T) {
	type S1 struct {
		Stream string
	}
	type S2 struct {
		Stream string `json:"stream"`
	}
	msg := []byte(`{"stream":"foo"}`)
	msg2 := []byte(`{"Stream":"foo"}`)

	var s1 S1
	err := json.Unmarshal(msg, &s1)
    if err != nil {
        log.Fatalln(err)
    }
	if s1.Stream != "foo" {
		log.Fatalln("not ok")
	}
	err = json.Unmarshal(msg2, &s1)
    if err != nil {
        log.Fatalln(err)
    }

	var s2 S2
	err = json.Unmarshal(msg2, &s2)
    if err != nil {
        log.Fatalln(err)
    }
	err = json.Unmarshal(msg, &s2)
    if err != nil {
        log.Fatalln(err)
    }
}
```

## sonic tag 匹配性能更好

在 sonic 源码中，如果用 json tag 完全匹配的话性能更好

反序列化缓存的map中就不会是"key"和"Key"两份数据

https://github.com/bytedance/sonic/blob/15dff369eda286b96646da1dfa41fb9f5cfdc5e3/internal/caching/fcache.go#L102-L103

```
1. select by the length: 0 ~ 32 and larger lengths
2. simd match the aligned prefix of the keys: 4/8/16/32 bytes or larger keys
3. check the key with strict match
4. check the key with case-insensitive match
5. find the index 

    /* add the case-insensitive version, prefer the one with smaller field ID */
    key := strings.ToLower(name)
    if v, ok := self.m[key]; !ok || i < v { // 如果纯小写格式相同,反序列化缓存的map中就不会是"key"和"Key"两份数据 性能更好
        self.m[key] = i
    }
```

## Go 源码大小写处理

既然 sonic 源码用的是 `strings.ToLower` 那么 Go 源码 `/home/w/go/src/encoding/json` 中居然找不到 `ToLower` 调用

换种搜索方法，大小写字母之间差 32 搜索 `32` 或者 `0x20` 也找不到

最后只能尝试搜索 'a' 或者 'A' 了, 找到 **`fold.go`** 中 **把所有小写字母减成大写字母**

```go
// foldName returns a folded string such that foldName(x) == foldName(y)
// is identical to bytes.EqualFold(x, y).
func foldName(in []byte) []byte {
	// This is inlinable to take advantage of "function outlining".
	var arr [32]byte // large enough for most JSON names
	return appendFoldedName(arr[:0], in)
}

func appendFoldedName(out, in []byte) []byte {
	for i := 0; i < len(in); {
		// Handle single-byte ASCII.
		if c := in[i]; c < utf8.RuneSelf {
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			out = append(out, c)
			i++
			continue
		}
		// Handle multi-byte Unicode.
		r, n := utf8.DecodeRune(in[i:])
		out = utf8.AppendRune(out, foldRune(r))
		i += n
	}
	return out
}
```

在 decode.go 中 如果 json tag 中的名字匹配失败，就需要额外的开销再调用 `foldName(key)` 转换成全大写去匹配

```go
func (d *decodeState) object(v reflect.Value) error {
    // ...
			f := fields.byExactName[string(key)]
			if f == nil {
				f = fields.byFoldedName[string(foldName(key))]
			}
}
```

首先为了能被json反射首字母大写 但为了 json 反序列更好的性能，无论用标准库还是 sonic 都应该用 json tag 例如 \`json:"data"`


