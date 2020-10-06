package cryptonym

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"golang.org/x/crypto/pbkdf2"
	"os"
	"time"
)

const (
	settingsDir      = "com.blockpane.cryptonym"
	settingsFileName = "cryptonym.dat"
)

type FioSettings struct {
	Server string `json:"server"`
	Proxy  string `json:"proxy"`

	DefaultKey     string `json:"default_key"`
	DefaultKeyDesc string `json:"default_key_desc"`
	FavKey2        string `json:"fav_key_2"`
	FavKey2Desc    string `json:"fav_key_2_desc"`
	FavKey3        string `json:"fav_key_3"`
	FavKey3Desc    string `json:"fav_key_3_desc"`
	FavKey4        string `json:"fav_key_4"`
	FavKey4Desc    string `json:"fav_key_4_desc"`

	MsigAccount string `json:"msig_account"`
	Tpid        string `json:"tpid"`

	AdvancedFeatures bool `json:"advanced_features"`

	// future:
	KeosdAddress  string `json:"keosd_address"`
	KeosdPassword string `json:"keosd_password"`
}

func DefaultSettings() *FioSettings {
	return &FioSettings{
		Server:         "http://127.0.0.1:8888",
		Proxy:          "http://127.0.0.1:8080",
		DefaultKey:     "5JBbUG5SDpLWxvBKihMeXLENinUzdNKNeozLas23Mj6ZNhz3hLS",
		DefaultKeyDesc: "devnet - vote 1",
		FavKey2:        "5KC6Edd4BcKTLnRuGj2c8TRT9oLuuXLd3ZuCGxM9iNngc3D8S93",
		FavKey2Desc:    "devnet - vote 2",
		FavKey3:        "5KQ6f9ZgUtagD3LZ4wcMKhhvK9qy4BuwL3L1pkm6E2v62HCne2R",
		FavKey3Desc:    "devnet - bp1",
		FavKey4:        "5HwvMtAEd7kwDPtKhZrwA41eRMdFH5AaBKPRim6KxkTXcg5M9L5",
		FavKey4Desc:    "devnet - locked 1",
		MsigAccount:    "eosio",
		Tpid:           "tpid@blockpane",
	}
}

func EncryptSettings(set *FioSettings, salt []byte, password string) (encrypted []byte, err error) {
	if password == "" {
		return nil, errors.New("invalid password supplied")
	}

	// if a salt isn't supplied, create one, note: using crypto/rand NOT math/rand, has better entropy
	if salt == nil || len(salt) != 12 || bytes.Equal(salt, bytes.Repeat([]byte{0}, 12)) {
		salt = make([]byte, 12)
		if _, e := rand.Read(salt); e != nil {
			errs.ErrChan <- "EncryptSettings: " + e.Error()
			return nil, err
		}
	}

	// prepend the salt to our buffer:
	crypted := bytes.NewBuffer(nil)
	crypted.Write(salt)

	// convert our settings to a binary struct
	data := bytes.NewBuffer(nil)
	g := gob.NewEncoder(data)
	err = g.Encode(set)
	if err != nil {
		errs.ErrChan <- "EncryptSettings: " + err.Error()
		return nil, err
	}

	// password-based key derivation, 48 bytes, 1st 32 is aes key, last 12 mac key
	key := pbkdf2.Key([]byte(password), salt, 12*1024, 48, sha256.New)

	// aes 256
	cb, err := aes.NewCipher(key[:32])
	if err != nil {
		errs.ErrChan <- "EncryptSettings: " + err.Error()
		return nil, err
	}

	//pkcs7 pad the plaintext
	plaintext := append(data.Bytes(), func() []byte {
		padLen := cb.BlockSize() - (len(data.Bytes()) % cb.BlockSize())
		pad := make([]byte, padLen)
		for i := range pad {
			pad[i] = uint8(padLen)
		}
		return pad
	}()...)

	// use an authenticated (Galois) cipher
	gcm, err := cipher.NewGCM(cb)
	if err != nil {
		errs.ErrChan <- "EncryptSettings: " + err.Error()
		return nil, err
	}
	// since the nonce should be secret and is derived via pbkdf, don't save it, write ciphertext directly to the buffer
	l, err := crypted.Write(gcm.Seal(nil, key[len(key)-gcm.NonceSize():], plaintext, nil))
	if err != nil {
		errs.ErrChan <- "EncryptSettings: " + err.Error()
		return nil, err
	} else if l < len(plaintext)+gcm.Overhead() {
		err = errors.New("unable to encrypt data, did not get correct size")
		errs.ErrChan <- "EncryptSettings: " + err.Error()
		return nil, err
	}

	// final paranoid sanity check that we got the correct amount of data back
	if len(crypted.Bytes()) != len(salt)+len(plaintext)+gcm.Overhead() {
		return nil, errors.New("unable to encrypt data, resulting ciphertext was wrong size")
	}

	return crypted.Bytes(), nil
}

func DecryptSettings(encrypted []byte, password string) (settings *FioSettings, err error) {
	key := pbkdf2.Key([]byte(password), encrypted[:12], 12*1024, 48, sha256.New)
	cb, err := aes.NewCipher(key[:32])
	if err != nil {
		errs.ErrChan <- "DecryptSettings: " + err.Error()
		return nil, err
	}
	gcm, err := cipher.NewGCM(cb)
	if err != nil {
		errs.ErrChan <- "DecryptSettings: " + err.Error()
		return nil, err
	}
	plain, err := gcm.Open(nil, key[len(key)-gcm.NonceSize():], encrypted[12:], nil)
	if err != nil {
		errs.ErrChan <- "DecryptSettings: " + err.Error()
		return nil, err
	}
	padLen := int(plain[len(plain)-1])
	if len(plain) <= padLen {
		err = errors.New("invalid padding, plaintext smaller than pkcs7 pad size")
		errs.ErrChan <- "DecryptSettings: " + err.Error()
		return nil, err
	}
	g := gob.NewDecoder(bytes.NewReader(plain[:len(plain)-padLen]))
	err = g.Decode(&settings)
	if err != nil {
		errs.ErrChan <- "DecryptSettings: " + err.Error()
		return nil, err
	}
	if settings.AdvancedFeatures {
		_ = os.Setenv("ADVANCED", "true")
	}
	return
}

const (
	settingsRead uint8 = iota
	settingsSave
)

// will never return a nil settings
func LoadEncryptedSettings(password string) (ok bool, fileLength int, settings *FioSettings, err error) {
	ok, encrypted, err := readWriteSettings(settingsRead, nil)
	if !ok {
		return ok, len(encrypted), DefaultSettings(), err
	}
	decrypted, err := DecryptSettings(encrypted, password)
	if err != nil {
		return false, len(encrypted), DefaultSettings(), err
	}
	if decrypted == nil {
		return false, len(encrypted), DefaultSettings(), errors.New("unknown error decrypting config, got empty config")
	}
	return true, len(encrypted), decrypted, nil
}

func SaveEncryptedSettings(password string, settings *FioSettings) (ok bool, err error) {
	encrypted, err := EncryptSettings(settings, nil, password)
	if err != nil {
		return false, err
	}
	ok, _, err = readWriteSettings(settingsSave, encrypted)
	return
}

func MkDir() (ok bool, err error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false, nil
	}
	dirName := fmt.Sprintf("%s%c%s", configDir, os.PathSeparator, settingsDir)
	var createDir bool
	dirStat, err := os.Stat(dirName)
	if _, ok := err.(*os.PathError); ok {
		if e := os.Mkdir(dirName, os.FileMode(0700)); e != nil {
			return false, err
		}
		createDir = true
	} else if err != nil {
		errs.ErrChan <- "MkDir: " + err.Error()
		return false, err
	}
	if dirStat == nil && !createDir {
		return false, errors.New("unknown error creating configuration directory")
	} else if !createDir && !dirStat.IsDir() {
		err = errors.New("cannot create directory, file already exists")
		errs.ErrChan <- "readWriteSettings: " + err.Error()
		return false, err
	}
	return true, nil
}

func readWriteSettings(action uint8, fileBytes []byte) (ok bool, content []byte, err error) {
	if action != settingsRead && action != settingsSave {
		return false, nil, errors.New("invalid action")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		errs.ErrChan <- err.Error()
		return false, nil, err
	}
	_, err = MkDir()
	if err != nil {
		return false, nil, err
	}

	fileName := fmt.Sprintf("%s%c%s%c%s", configDir, os.PathSeparator, settingsDir, os.PathSeparator, settingsFileName)
	f := &os.File{}
	fileStat, err := os.Stat(fileName)
	if _, ok := err.(*os.PathError); ok {
		if action == settingsRead {
			return false, nil, nil
		}
	} else if err != nil {
		return false, nil, err
	}

	// handle request to read file:
	if fileStat != nil && fileStat.Size() > 0 && action == settingsRead {
		f, err := os.OpenFile(fileName, os.O_RDONLY, 600)
		if err != nil {
			return false, nil, err
		}
		defer f.Close()
		contents := make([]byte, int(fileStat.Size()))
		n, err := f.Read(contents)
		if err != nil {
			return false, nil, err
		}
		if int64(n) != fileStat.Size() {
			return false, nil, errors.New("could not read file, truncated result")
		}
		return true, contents, nil
	} else if action == settingsRead {
		return false, nil, nil
	}

	// otherwise write the new config file:
	f, err = os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return false, nil, err
	}
	defer f.Close()
	_ = f.SetWriteDeadline(time.Now().Add(time.Second))
	n, err := f.Write(fileBytes)
	if err != nil {
		return false, nil, err
	}
	if n != len(fileBytes) {
		return false, nil, errors.New("did not write entire file, output truncated")
	}
	return true, nil, nil
}
