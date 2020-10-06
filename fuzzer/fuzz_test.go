package fuzzer

import (
	"encoding/hex"
	"fmt"
	"github.com/fioprotocol/fio-go"
	"math/rand"
	"strings"
	"testing"
)

func TestRandomString(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := RandomString(17)
		fmt.Println(s)
		if len(s) < 16 {
			t.Error("too short")
		}
	}
}

func TestRandomActor(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := RandomActor()
		fmt.Println(s)
		if len(s) != 12 {
			t.Error("wrong size")
		}
	}
}

func TestRandomFioPubKey(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := RandomFioPubKey()
		fmt.Println(s)
		if len(s) != 53 {
			t.Error("wrong size")
		}
	}
}

func TestRandomBytes(t *testing.T) {
	for _, enc := range []int8{EncodeBase64, EncodeHexString, EncodeRaw} {
		for i := 0; i < 10; i++ {
			s := RandomBytes(rand.Intn(112)+16, enc)
			fmt.Println(s)
			if s == "" {
				t.Error("empty!")
			}
		}
	}
}

func TestRandomChecksum(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := RandomChecksum()
		fmt.Println(s)
		if len(s) != 64 {
			t.Error("wrong size for checksum256")
		}
	}
}

func TestHexToBytes(t *testing.T) {
	for i := 0; i < 10; i++ {
		b := make([]byte, 32)
		rand.Read(b)
		s := HexToBytes(hex.EncodeToString(b))
		if string(b) != s {
			t.Error("bytes didn't match")
			continue
		}
		fmt.Println(s, " == ", string(b))
	}
}

func TestChecksumOf(t *testing.T) {
	for i := 0; i < 10; i++ {
		b := make([]byte, rand.Intn(128)+32)
		s := ChecksumOf(string(b))
		fmt.Println(s)
		if len(s) != 64 {
			t.Error("wrong size")
		}
	}
}

func TestSignatureFor(t *testing.T) {
	key, _ := fio.NewRandomAccount()
	for i := 0; i < 10; i++ {
		b := make([]byte, rand.Intn(128)+32)
		s := SignatureFor(string(b), key)
		fmt.Println(s)
		if !strings.HasPrefix(s, "SIG_K1_") {
			t.Error("invalid signature")
		}
	}
}
