package cryptonym

import (
	"fmt"
	"testing"
)

func TestEncryptSettings(t *testing.T) {
	s := DefaultSettings()
	encrypted, err := EncryptSettings(s, nil, "password")
	if err != nil {
		t.Error(err)
		return
	}
	decrypted, err := DecryptSettings(encrypted, "password")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("%#v\n", decrypted)

}
