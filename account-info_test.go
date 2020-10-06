package cryptonym

import (
	"encoding/json"
	"fmt"
	"testing"
)

/*
	"Actor/Account",
	"Public Key",
	"Private Key",
	"Fio Address",
	"Fio Domain"

*/
func TestAccountSearch(t *testing.T) {
	pub := "FIO6G9pXXM92Gy5eMwNquGULoCj3ZStwPLPdEb9mVXyEHqWN7HSuA"
	priv := "5JBbUG5SDpLWxvBKihMeXLENinUzdNKNeozLas23Mj6ZNhz3hLS"
	actor := "o2ouxipw2rt4"
	address := "vote1@dapixdev"

	Uri = "http://localhost:8888"

	a, e := AccountSearch("Fio Address", address)
	if e != nil {
		t.Error(e.Error())
	} else {
		j, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println("address:")
		fmt.Println(string(j))
		fmt.Println("")
	}

	a, e = AccountSearch("Private Key", priv)
	if e != nil {
		t.Error(e.Error())
	} else {
		j, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println("priv:")
		fmt.Println(string(j))
		fmt.Println("")
	}

	a, e = AccountSearch("Public Key", pub)
	if e != nil {
		t.Error(e.Error())
	} else {
		j, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println("pub:")
		fmt.Println(string(j))
		fmt.Println("")
	}

	a, e = AccountSearch("Actor/Account", actor)
	if e != nil {
		t.Error(e.Error())
	} else {
		j, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println("actor:")
		fmt.Println(string(j))
		fmt.Println("")
	}
}
