package cryptonym

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/blockpane/cryptonym/fuzzer"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// TODO: rethink this, instead of calling this every request, maybe pass pointers to functions?
func (abi *Abi) GeneratePayloads(key *fio.Account) error {
	abi.mux.RLock()
	defer abi.mux.RUnlock()

	for i := 0; i < len(abi.Rows); i++ {
		form := &abi.Rows[i]
		err := func() error {
			var v interface{}
			v = nil
			isSlice := false
			if strings.HasSuffix(form.Type.Selected, `[]`) {
				isSlice = true
			}
			switch form.SendAs.Selected {

			case "form value":
				switch form.Variation.Selected {
				case "as is":
					if strings.Contains(form.Type.Selected, "int") {
						t, e := strconv.ParseInt(form.Input.Text, 10, 64)
						if e != nil {
							return errors.New(*form.Name + ": " + e.Error())
						}
						abi.mux.RUnlock()
						FormState.UpdateValue(&i, t, isSlice, true)
						abi.mux.RLock()
						return nil
					} else if strings.Contains(form.Type.Selected, "float") {
						t, e := strconv.ParseFloat(form.Input.Text, 64)
						if e != nil {
							return errors.New(*form.Name + ": " + e.Error())
						}
						abi.mux.RUnlock()
						FormState.UpdateValue(&i, t, isSlice, true)
						abi.mux.RLock()
						return nil
					}
					v = form.Input.Text
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "FIO -> suf":
					unFriendly := strings.ReplaceAll(strings.ReplaceAll(form.Input.Text, ",", ""), "_", "")
					f, e := strconv.ParseFloat(unFriendly, 64)
					if e != nil {
						return errors.New(*form.Name + ": " + e.Error())
					}
					t := uint64(f * 1_000_000_000.0)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, t, isSlice, true)
					abi.mux.RLock()
					return nil
				case "json -> struct":
					v = form.Input.Text
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, false, true)
					abi.mux.RLock()
					return nil
				case "hex -> byte[]":
					h, e := hex.DecodeString(form.Input.Text)
					if e != nil {
						return errors.New(*form.Name + ": " + e.Error())
					}
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, h, isSlice, false)
					abi.mux.RLock()
					return nil
				case "base64 -> byte[]":
					buf := bytes.NewReader([]byte(form.Input.Text))
					b64 := base64.NewDecoder(base64.StdEncoding, buf)
					b := make([]byte, 0)
					_, e := b64.Read(b)
					if e != nil {
						return errors.New(*form.Name + ": " + e.Error())
					}
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, b, isSlice, false)
					abi.mux.RLock()
					return nil
				case "checksum256":
					v = fuzzer.ChecksumOf(form.Input.Text)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "signature":
					v = fuzzer.SignatureFor(form.Input.Text, key)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "fio address@ (valid)":
					v = fuzzer.FioAddressAt(form.Input.Text)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
				case "fio address@ (valid, max size)":
					v = fuzzer.MaxRandomFioAddressAt(form.Input.Text)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
				case "fio address@ (invalid)":
					v = fuzzer.InvalidFioAddressAt(form.Input.Text)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
				}

			case "actor":
				switch form.Variation.Selected {
				case "mine":
					v = key.Actor
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "random":
					v = fuzzer.RandomActor()
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				}

			case "pub key":
				switch form.Variation.Selected {
				case "mine":
					v = key.PubKey
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "random":
					v = fuzzer.RandomFioPubKey()
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				}

			case "fio types":
				switch form.Variation.Selected {
				// TODO:
				//case "random array of existing fio address":
				case "max length: addaddress.public_addresses":
					v = fuzzer.MaxAddPubAddress()
					fmt.Println(v)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, false, true)
					abi.mux.RLock()
					return nil
				case "max length: voteproducer.producers":
					v = fuzzer.MaxVoteProducers(Uri)
					fmt.Println(v)
					abi.mux.RUnlock()
					FormState.UpdateValueWithConvert(&i, v, false, "string[]", false)
					abi.mux.RLock()
					return nil
				case "variable length: addaddress.public_addresses":
					var payloadLen int
					var err error
					if form.Len.Selected == "" {
						payloadLen = 1
						form.Len.SetSelected("1")
					} else {
						payloadLen, err = strconv.Atoi(form.Len.Selected)
						if err != nil {
							payloadLen = 1
						}
					}
					v = fuzzer.RandomAddAddress(payloadLen)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, false, true)
					abi.mux.RLock()
					return nil
				case "invalid fio domain":
					v = fuzzer.InvalidFioDomain()
				case "valid fio domain (max size)":
					v = fuzzer.MaxRandomFioDomain()
				case "valid fio domain":
					v = fuzzer.FioDomain()
				case "max length: newfundsreq.content":
					v = fuzzer.MaxNewFundsContent()
				case "max length: recordobt.content":
					v = fuzzer.MaxRecObtContent()
				case "max length: regproducer.url":
					v = fuzzer.MaxProducerUrl()
				case "random existing fio address":
					v = fuzzer.RandomExistingFioAddress(Uri)
				}
				abi.mux.RUnlock()
				FormState.UpdateValue(&i, v, isSlice, false)
				abi.mux.RLock()
				return nil

			case "number":
				var l int
				var e error
				if form.Variation.Selected != "random number (mixed)" && form.Variation.Selected != "max int" {
					if form.Len.Selected == "" {
						return errors.New(*form.Name + ": no number specified")
					}
					l, e = strconv.Atoi(form.Len.Selected)
					if e != nil {
						return errors.New(*form.Name + ": " + e.Error())
					}
				}
				var noJsonEscape = true
				switch form.Variation.Selected {
				case "max int":
					abiType := form.Len.Selected
					if form.Type.Selected == "string" {
						abiType = "string"
						noJsonEscape = false
					}
					v = fuzzer.MaxInt(form.Len.Selected)
					abi.mux.RUnlock()
					//FormState.UpdateValue(&i, v, false, true)
					FormState.UpdateValueWithConvert(&i, v, isSlice, abiType, noJsonEscape)
					abi.mux.RLock()
					return nil
				case "incrementing float":
					abiType := "float64"
					if form.Type.Selected == "string" {
						abiType = "string"
						noJsonEscape = false
					}
					v = fuzzer.IncrementingFloat()
					abi.mux.RUnlock()
					FormState.UpdateValueWithConvert(&i, v, isSlice, abiType, noJsonEscape)
					abi.mux.RLock()
					return nil
				case "incrementing int":
					abiType := "int64"
					if form.Type.Selected == "string" {
						abiType = "string"
						noJsonEscape = false
					}
					v = fuzzer.IncrementingInt()
					abi.mux.RUnlock()
					FormState.UpdateValueWithConvert(&i, v, isSlice, abiType, noJsonEscape)
					abi.mux.RLock()
					return nil
				case "random float":
					abiType := fmt.Sprintf("float%d", l)
					if form.Type.Selected == "string" {
						abiType = "string"
						noJsonEscape = false
					}
					v = fuzzer.RandomFloat(l)
					abi.mux.RUnlock()
					FormState.UpdateValueWithConvert(&i, v, isSlice, abiType, noJsonEscape)
					abi.mux.RLock()
					return nil
				case "random int":
					switch l {
					case 128:
						v = fuzzer.RandomInt128()
						abi.mux.RUnlock()
						FormState.UpdateValueWithConvert(&i, v, isSlice, "string", false)
					default:
						abiType := fmt.Sprintf("int%d", l)
						if form.Type.Selected == "string" {
							abiType = "string"
							noJsonEscape = false
						}
						v = fuzzer.RandomInteger(l).Interface()
						abi.mux.RUnlock()
						FormState.UpdateValueWithConvert(&i, v, isSlice, abiType, noJsonEscape)
					}
					abi.mux.RLock()
					return nil
				case "overflow int":
					abiType := "uint64"
					if form.Type.Selected == "string" {
						abiType = "string"
						noJsonEscape = false
					}
					signed := false
					if strings.HasPrefix(form.Type.Selected, "int") {
						signed = true
					}
					v = fuzzer.OverFlowInt(l, signed)
					abi.mux.RUnlock()
					FormState.UpdateValueWithConvert(&i, v, isSlice, abiType, noJsonEscape)
					abi.mux.RLock()
					return nil
				case "random number (mixed)":
					rn := fuzzer.RandomNumber()
					abiType := rn.AbiType()
					if form.Type.Selected == "string" {
						abiType = "string"
						noJsonEscape = false
					}
					abi.mux.RUnlock()
					FormState.UpdateValueWithConvert(&i, rn.String(), false, abiType, noJsonEscape)
					abi.mux.RLock()
					return nil
				}

			case "bytes/string":
				var hasLen bool
				var payloadLen int
				if form.Len.Selected != "" {
					hasLen = true
					e := func() error {
						if form.Len.Selected == "random length" {
							payloadLen = rand.Intn(math.MaxInt16 + 8)
							return nil
						}
						var e error
						payloadLen, e = strconv.Atoi(strings.ReplaceAll(form.Len.Selected, ",", ""))
						if e != nil {
							return errors.New(*form.Name + ": invalid number for payload length")
						}
						return nil
					}()
					if e != nil {
						return e
					}
				}
				lenError := func() error {
					return errors.New(*form.Name + ": no length specified for random payload")
				}
				switch form.Variation.Selected {
				case "string":
					if !hasLen {
						return lenError()
					}
					v = fuzzer.RandomString(payloadLen)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "bytes":
					if !hasLen {
						return lenError()
					}
					v = fuzzer.RandomBytes(payloadLen, fuzzer.EncodeRaw)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "bytes: hex encoded":
					if !hasLen {
						return lenError()
					}
					v = fuzzer.RandomBytes(payloadLen, fuzzer.EncodeHexString)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "bytes: base64 encoded":
					if !hasLen {
						return lenError()
					}
					v = fuzzer.RandomBytes(payloadLen, fuzzer.EncodeBase64)
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				case "random checksum":
					v = fuzzer.RandomChecksum()
					abi.mux.RUnlock()
					FormState.UpdateValue(&i, v, isSlice, false)
					abi.mux.RLock()
					return nil
				}

			case "load file":
				return errors.New("load file is not implemented yet")

			default:
				return errors.New("unknown generator provided")
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func (abi *Abi) PackAndSign(api *fio.API, opts *fio.TxOptions, account *fio.Account, msig bool) (json.RawMessage, *eos.PackedTransaction, error) {
	if len(abi.Rows) == 0 {
		return nil, nil, nil
	}
	abi.mux.RLock()
	defer abi.mux.RUnlock()

	// build a json string
	jsonString := "{"
	for i, row := range abi.Rows {
		if row.convert == nil {
			row.convert = func(s interface{}) interface{} {
				return s
			}
		}
		b := make([]byte, 0)
		var err error
		switch {
		case !row.IsSlice && !row.noJsonEscape:
			b, err = json.Marshal(row.convert(row.Value))
			if err != nil {
				errs.ErrChan <- err.Error()
			}
		case !row.IsSlice && row.noJsonEscape:
			b = []byte(fmt.Sprintf("%v", *row.Value))
		//FIXME, this is converting everything to string arrays
		case row.IsSlice:
			payloads := func() []string {
				p := strings.Split(fmt.Sprintf("%v", row.Input.Text), ",")
				s := make([]string, len(p))
				for i := range p {
					s[i] = strings.TrimSpace(p[i])
				}
				return s
			}()
			b, err = json.Marshal(payloads)
			if err != nil {
				errs.ErrChan <- err.Error()
			}
		}
		jsonString = jsonString + fmt.Sprintf(`"%s":%s`, *row.Name, string(b))
		if i < len(abi.Rows)-1 {
			jsonString = jsonString + ","
		}
	}
	jsonString = jsonString + "}"

	rawJ := json.RawMessage([]byte(jsonString))
	// make sure we can marshall the json we just created ...
	if len(jsonString) >= 524288 {
		rawJ = []byte(fmt.Sprintf(`{"message":"Not showing request: %d bytes is too large"}"`, len(jsonString)))
	} else {
		err := json.Unmarshal([]byte(jsonString), &rawJ)
		if err != nil {
			return nil, nil, errors.New("could not marshal new data into json: " + err.Error())
		}
	}

	// get the "real" abi, and we will update it with any changes:
	newAbi, err := api.GetABI(eos.AccountName(abi.Rows[0].Contract))
	if err != nil {
		return nil, nil, err
	}
	newStructBytes := abi.DeriveJsonAbi()
	newDef := eos.StructDef{}
	err = json.Unmarshal(newStructBytes, &newDef)
	for i, def := range newAbi.ABI.Structs {
		if def.Name == abi.Action {
			newAbi.ABI.Structs[i] = newDef
			break
		}
	}

	encoded, err := newAbi.ABI.EncodeAction(eos.ActionName(abi.Action), []byte(jsonString))
	if err != nil {
		return nil, nil, err
	}

	actionData := eos.NewActionData(nil)
	actionData.HexData = encoded
	var finalActor string
	for _, r := range abi.Rows {
		if *r.Name == "actor" {
			finalActor = fmt.Sprintf("%v", *r.Value)
		}
	}
	if finalActor == "" {
		finalActor = string(account.Actor)
	}
	action := &fio.Action{
		Account: eos.AccountName(abi.Contract),
		Name:    eos.ActionName(abi.Action),
		Authorization: []eos.PermissionLevel{
			{
				Actor:      eos.AccountName(finalActor),
				Permission: "active",
			},
		},
		ActionData: actionData,
	}
	compression := fio.CompressionNone
	if useZlib {
		compression = fio.CompressionZlib
	}
	opts.TxOptions.DelaySecs = 0
	if deferTx {
		opts.TxOptions.DelaySecs = uint32(delayTxSec)
	}
	signMe := fio.NewTransaction([]*fio.Action{action}, opts)
	if msig {
		signMe.Expiration.Time = time.Now().Add(time.Hour)
	}
	_, packedTx, err := api.SignTransaction(
		signMe,
		opts.ChainID,
		compression,
	)
	if err != nil {
		return nil, nil, err
	}

	return rawJ, packedTx, nil
}
