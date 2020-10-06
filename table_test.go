package cryptonym

import (
	"fmt"
	"github.com/fioprotocol/fio-go"
	"testing"
)

func TestGetAccountActions(t *testing.T) {
	api, _, err := fio.NewConnection(nil, "http://127.0.0.1:8888")
	if err != nil {
		t.Error(err)
		return
	}
	api.Header.Set("User-Agent", "fio-cryptonym-wallet")
	a, err := GetAccountSummary(api)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(a)
}
