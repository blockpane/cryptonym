package fuzzer

import (
	"bytes"
	"context"
	cr "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"github.com/fioprotocol/fio-go/eos/ecc"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

const (
	EncodeRaw int8 = iota
	EncodeHexString
	EncodeBase64
	badChars = `~!#$%^&*()+=\|{}[]";:?/.>,<@"` + "`"
)

func RandomString(length int) string {
	var payload string
	for i := 0; i < length; i++ {
		payload = payload + string(byte(rand.Intn(26)+97))
	}
	return payload
}

type RandomNumberResult struct {
	abi     string
	value   interface{}
	convert func(s interface{}) interface{}
}

func (rn RandomNumberResult) AbiType() string {
	return rn.abi
}

func (rn RandomNumberResult) String() string {
	return fmt.Sprintf("%v", rn.value)
}

func (rn RandomNumberResult) Interface() interface{} {
	return rn.value
}

func (rn RandomNumberResult) ConvertFunc() func(s interface{}) interface{} {
	return rn.convert
}

func RandomNumber() RandomNumberResult {
	intLens := [...]int{8, 16, 32, 64} // don't send 128 here.
	floatLen := 32

	var result interface{}
	signed := false
	rn := RandomNumberResult{}

	negative := 1
	if rand.Intn(2) > 0 {
		signed = true
		if rand.Intn(2) > 0 {
			negative = -1
		}
	}

	intOrFloat := rand.Intn(2)
	switch intOrFloat {
	case 0:
		l := intLens[rand.Intn(len(intLens))]
		result = RandomInteger(l)
		rn.abi = fmt.Sprintf("int%d", l)
		if !signed {
			rn.abi = fmt.Sprintf("uint%d", l)
		}
		switch l {
		case 8:
			rn.convert = func(s interface{}) interface{} {
				parsed, _ := strconv.ParseInt(fmt.Sprintf("%d", s), 10, 8)
				if !signed {
					return uint8(parsed)
				}
				return int8(parsed * int64(negative))
			}
		case 16:
			rn.convert = func(s interface{}) interface{} {
				parsed, _ := strconv.ParseInt(fmt.Sprintf("%d", s), 10, 16)
				if !signed {
					return uint16(parsed)
				}
				return int16(parsed * int64(negative))
			}
		case 32:
			rn.convert = func(s interface{}) interface{} {
				parsed, _ := strconv.ParseInt(fmt.Sprintf("%d", s), 10, 32)
				if !signed {
					return uint32(parsed)
				}
				return int32(parsed * int64(negative))
			}
		case 64:
			rn.convert = func(s interface{}) interface{} {
				parsed, _ := strconv.ParseInt(fmt.Sprintf("%d", s), 10, 64)
				if !signed {
					return uint64(parsed)
				}
				return parsed * int64(negative)
			}
		}
	case 1:
		fl := floatLen * (rand.Intn(2) + 1)
		if fl == 32 {
			rn.abi = "float32"
			result = float32(RandomFloat(fl) * float64(negative))
		} else {
			rn.abi = "float64"
			result = RandomFloat(fl) * float64(negative)
		}

	}
	rn.value = result
	return rn
}

func MaxInt(size string) uint64 {
	switch size {
	case "int8":
		return uint64(math.MaxInt8)
	case "uint8":
		return uint64(math.MaxUint8)
	case "int16":
		return uint64(math.MaxInt16)
	case "uint16":
		return uint64(math.MaxUint16)
	case "int32":
		return uint64(math.MaxInt32)
	case "uint32":
		return uint64(math.MaxUint32)
	case "int64":
		return uint64(math.MaxInt64)
	case "uint64":
		return math.MaxUint64
	default:
		return uint64(math.MaxInt32)
	}
}

func RandomInteger(size int) RandomNumberResult {
	switch size {
	case 8:
		//return int8(rand.Intn(math.MaxInt8-1) + 1)
		return RandomNumberResult{
			abi:   "int8",
			value: int8(rand.Intn(math.MaxInt8-1) + 1),
		}
	case 16:
		return RandomNumberResult{
			abi:   "int16",
			value: int16(rand.Intn(math.MaxInt16-1) + 1),
		}
	case 32:
		return RandomNumberResult{
			abi:   "int32",
			value: int32(rand.Intn(math.MaxInt32-1) + 1),
		}
	case 64:
		return RandomNumberResult{
			abi:   "int64",
			value: int64(rand.Intn(math.MaxInt64-1) + 1),
		}
	}
	return RandomNumberResult{
		abi:   "int32",
		value: int32(rand.Intn(math.MaxInt16-1)+1) * -1,
	}
}

func OverFlowInt(size int, signed bool) string {
	switch {
	case size == 8 && signed:
		return fmt.Sprintf("%d", int16(math.MaxInt8)+1)
	case size == 16 && signed:
		return fmt.Sprintf("%d", int32(math.MaxInt16)+1)
	case size == 32 && signed:
		return fmt.Sprintf("%d", int64(math.MaxInt32)+1)
	case size == 8 && !signed:
		return fmt.Sprintf("%d", uint16(math.MaxUint8)+1)
	case size == 16 && !signed:
		return fmt.Sprintf("%d", uint32(math.MaxUint16)+1)
	case size == 32 && !signed:
		return fmt.Sprintf("%d", uint64(math.MaxUint32)+1)
	}
	return ""
}

func RandomInt128() string {
	j, _ := eos.Int128{
		Lo: rand.Uint64(),
		Hi: rand.Uint64(),
	}.MarshalJSON()
	return string(j)
}

func RandomFloat(size int) float64 {
	switch size {
	case 32:
		f := rand.Float32()
		if f < .4 {
			f = f + rand.Float32()*1000.0
		}
		return float64(f)
	case 64:
		f := rand.Float64()
		if f < .4 {
			f = f + rand.Float64()*10000.0
			// if we are sending a f64, make sure it's a big one.
			if f < math.MaxFloat32 {
				f = f + float64(math.MaxInt16+rand.Intn(math.MaxInt16))
			}
		}
		return f
	}
	return 0.0
}

func RandomBytes(size int, encode int8) interface{} {
	var pl []byte
	// reading MBs of data from urand is a bad idea, use math.rand instead
	if size <= 4096 {
		pl = make([]byte, size)
		_, _ = cr.Read(pl)
	} else {
		pl = make([]byte, size)
		_, _ = rand.Read(pl)
	}
	switch encode {
	case EncodeRaw:
		return string(pl)
	case EncodeHexString:
		return hex.EncodeToString(pl)
	case EncodeBase64:
		buf := bytes.NewBuffer([]byte{})
		b64 := base64.NewEncoder(base64.StdEncoding, buf)
		_, e := b64.Write(pl)
		if e != nil {
			nonBlockErr("warning: error creating base64 - " + e.Error())
		}
		return string(buf.Bytes())
	}
	return ""
}

func RandomChecksum() eos.Checksum256 {
	cs := make([]byte, 32)
	_, _ = cr.Read(cs)
	return cs
}

func HexToBytes(hexData string) string {
	b, e := hex.DecodeString(hexData)
	if e != nil {
		nonBlockErr("warning: could not decode hex data, sending empty string")
		return ""
	}
	return string(b)
}

func ChecksumOf(value string) string {
	if value == "" {
		nonBlockErr("warning: sending checksum256 of an empty string")
	}
	sum := sha256.New()
	sum.Write([]byte(value))
	return hex.EncodeToString(sum.Sum(nil))
}

func SignatureFor(value string, key *fio.Account) string {
	hash, err := hex.DecodeString(ChecksumOf(value))
	if err != nil {
		nonBlockErr("couldn't hash value: " + err.Error())
		if len(hash) == 0 {
			return ""
		}
	}
	sig, err := key.KeyBag.Keys[0].Sign(hash)
	if err != nil {
		nonBlockErr("couldn't hash value: " + err.Error())
		return ""
	}
	return sig.String()
}

func word() string {
	var w string
	for i := 0; i < 6; i++ {
		w = w + string(byte(rand.Intn(26)+97))
	}
	return w
}

var incrementingInt int64

func IncrementingInt() int64 {
	incrementingInt = incrementingInt + 1
	return incrementingInt
}

var incrementingFloat float64

func IncrementingFloat() float64 {
	incrementingFloat = incrementingFloat + 1.00001
	return incrementingFloat
}

func ResetIncrement() {
	incrementingInt = 0
	incrementingFloat = 0.0
}

func RandomAddAddress(count int) string {
	addresses := make([]string, 0)
	for i := 0; i < count; i++ {
		code := word()
		addresses = append(addresses, fmt.Sprintf(`{"token_code": "%s", "chain_code": "%s", "public_address": "%s"}`, code, code, RandomBytes(32, EncodeHexString)))
	}
	return fmt.Sprintf(`[%s]`, strings.Join(addresses, ", "))
}

func NewPubAddress(user *fio.Account) (address string, chain string) {
	r := rand.Intn(3)
	switch r {
	case 0:
		chain = "BTC"
		wif, _ := btcutil.DecodeWIF(user.KeyBag.Keys[0].String())
		btcutil.Hash160(wif.PrivKey.PubKey().SerializeUncompressed())
		a, _ := btcutil.NewAddressPubKeyHash(btcutil.Hash160(wif.PrivKey.PubKey().SerializeUncompressed()), &chaincfg.MainNetParams)
		address = a.String()
	case 1:
		chain = "ETH"
		epk, _ := ecc.NewPublicKey("FIO" + user.PubKey[3:])
		pk, _ := epk.Key()
		address = crypto.PubkeyToAddress(*pk.ToECDSA()).String()
	case 2:
		chain = "EOS"
		address = "EOS" + user.PubKey[3:]
	}
	return
}

//// FIXME!
//func LoadFile(filename string) string {
//	nonBlockErr("warning: LoadFile is not implemented yet, sending empty string!")
//	return ""
//}

// try to notify on errors, but don't deadlock since we aren't entirely sure this is running inside the app
func nonBlockErr(msg string) {
	d := time.Now().Add(50 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()
	go func(s string) {
		errs.ErrChan <- s
	}(msg)
	select {
	case <-ctx.Done():
		return
	}
}
