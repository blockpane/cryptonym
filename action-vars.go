package cryptonym

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/blockpane/cryptonym/fuzzer"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

func abiSelectTypes(mustExist string) []string {
	types := []string{
		"authority",
		"bool",
		"byte",
		"byte[]",
		"checksum256",
		"float128",
		"float32",
		"float64",
		"hex_bytes",
		"int128",
		"int16",
		"int32",
		"int64",
		"name",
		"public_key",
		"signature",
		"string",
		"string[]",
		"symbol",
		"time",
		"timestamp",
		"uint128",
		"uint16",
		"uint32",
		"uint64",
		"varint32",
		"varuint32",
	}
	for _, t := range types {
		if t == mustExist {
			return types
		}
	}
	types = append(types, mustExist)
	return types
}

var sendAsSelectTypes = []string{
	"form value",
	"actor",
	"pub key",
	"fio types",
	"number",
	"bytes/string",
	//"load file",
}

var bytesVar = []string{
	"bytes",
	"bytes: base64 encoded",
	"bytes: hex encoded",
	"random checksum",
	"string",
}

var bytesLen = []string{
	"random length",
	"8",
	"12",
	"16",
	"32",
	"64",
	"128",
	"256",
	"512",
	"2,048",
	"4,096",
	//"8,192",
	//"16,384",
	//"32,768",
	//"65,536",
	//"131,072",
	//"262,144",
	//"524,288",
	//"1,048,576",
	//"2,097,152",
	//"4,194,304",
	//"8,388,608",
	//"16,777,216",
}

var formVar = []string{
	"as is",
	"FIO -> suf",
	"json -> struct",
	"base64 -> byte[]",
	"checksum256",
	"fio address@ (invalid)",
	"fio address@ (valid)",
	"fio address@ (valid, max size)",
	"hex -> byte[]",
	"signature",
}

var actorVar = []string{
	"mine",
	"random",
}

var numericVar = []string{
	"incrementing float",
	"incrementing int",
	"random float",
	"random int",
	"overflow int",
	"random number (mixed)",
	"max int",
}

var maxIntVar = []string{
	"int8",
	"uint8",
	"int16",
	"uint16",
	"int32",
	"uint32",
	"int64",
	"uint64",
}

var fioVar = []string{
	"invalid fio domain",
	"valid fio domain",
	"valid fio domain (max size)",
	"max length: newfundsreq.content",
	"max length: recordobt.content",
	"max length: regproducer.url",
	"max length: voteproducer.producers",
	"max length: addaddress.public_addresses",
	"variable length: addaddress.public_addresses",
	//TODO:
	//"string[] of existing fio address",
}

//TODO: "string[] of existing fio address"....
var addressLen = []string{
	"2",
	"4",
	"8",
	"16",
	"32",
}

var floatLen = []string{
	"32",
	"64",
}

var intLen = []string{
	"8",
	"16",
	"32",
	"64",
	"128",
}

var overflowLen = []string{
	"8",
	"16",
	"32",
}

var numAddresses = []string{
	"1",
	"2",
	"3",
	"4",
	"5",
	"10",
	"50",
	"100",
	"1000",
}

func sendAsVariant(kind string) (options []string, selected string) {
	switch kind {
	case "form value":
		return formVar, "as is"
	case "actor":
		return actorVar, "mine"
	case "pub key":
		return actorVar, "mine"
	case "number":
		return numericVar, "random int"
	case "bytes/string":
		return bytesVar, "string"
	case "fio types":
		return fioVar, "invalid fio domain"
	}
	return []string{}, "--"
}

func getLength(what string) (show bool, values []string, selected string) {
	switch {
	case what == "random float":
		return true, floatLen, "32"
	case what == "variable length addaddress.public_addresses":
		return true, numAddresses, "1"
	case what == "random int":
		return true, intLen, "32"
	case what == "overflow int":
		return true, overflowLen, "16"
	case what == "max int":
		return true, maxIntVar, "int32"
	case what == "random number (mixed)":
		return false, []string{""}, ""
	case strings.HasPrefix(what, "string") ||
		strings.HasPrefix(what, "bytes") ||
		strings.HasPrefix(what, "nop") ||
		strings.HasPrefix(what, "many"):
		return true, bytesLen, "64"
	}
	return
}

func defaultValues(contract string, action string, fieldName string, fieldType string, account *fio.Account, api *fio.API) string {
	var returnValue string
	switch {
	case fieldName == "amount":
		return "1,000.00"
	case fieldName == "bundled_transactions":
		return "100"
	case fieldName == "max_fee":
		api2, _, err := fio.NewConnection(nil, api.BaseURL)
		if err != nil {
			return "0"
		}
		api2.Header.Set("User-Agent", "fio-cryptonym-wallet")
		fio.UpdateMaxFees(api2)
		fee := fio.GetMaxFeeByAction(action)
		if fee == 0 {
			// as expensive as it gets ... pretty safe to return
			fee = fio.GetMaxFee("register_fio_domain")
		}
		returnValue = p.Sprintf("%.9f", fee)
	case fieldName == "can_vote":
		returnValue = "1"
	case fieldName == "is_public":
		returnValue = "1"
	case fieldType == "tokenpubaddr[]":
		a, t := fuzzer.NewPubAddress(account)
		returnValue = fmt.Sprintf(`[{
    "token_code": "%s",
    "chain_code": "%s",
    "public_address": "%s"
}]`, t, t, a)
	case fieldName == "url":
		returnValue = "https://fioprotocol.io"
	case fieldName == "location":
		returnValue = "80"
	case fieldName == "fio_domain":
		returnValue = "cryptonym"
	case fieldType == "bool":
		returnValue = "true"
	case fieldType == "authority":
		returnValue = `{
    "threshold": 2,
    "keys": [],
    "waits": [],
    "accounts": [
      {
        "permission": {
          "actor": "npe3obkgoteh",
          "permission": "active"
        },
        "weight": 1
      },
      {
        "permission": {
          "actor": "extjnqh3j3gt",
          "permission": "active"
        },
        "weight": 1
      }
    ]
  }`
	case strings.HasSuffix(fieldType, "int128"):
		i28 := eos.Uint128{
			Lo: uint64(rand.Int63n(math.MaxInt64)),
			Hi: uint64(rand.Int63n(math.MaxInt64)),
		}
		j, _ := json.Marshal(i28)
		returnValue = strings.Trim(string(j), `"`)
	case strings.HasPrefix(fieldType, "uint") || strings.HasPrefix(fieldType, "int"):
		returnValue = strconv.Itoa(rand.Intn(256))
	case strings.HasPrefix(fieldType, "float"):
		returnValue = "3.14159265359"
	case fieldName == "owner" || fieldName == "account" || fieldName == "actor" || fieldType == "authority" || fieldName == "proxy":
		actor, _ := fio.ActorFromPub(account.PubKey)
		returnValue = string(actor)
	case strings.Contains(fieldName, "public") || strings.HasSuffix(fieldName, "_key"):
		returnValue = account.PubKey
	case fieldName == "tpid":
		returnValue = Settings.Tpid
	case strings.HasSuffix(fieldName, "_address") || strings.HasPrefix(fieldName, "pay"):
		returnValue = DefaultFioAddress
	case fieldName == "authority" || (fieldName == "permission" && fieldType == "name"):
		returnValue = "active"
	case fieldName == "producers":
		returnValue = GetCurrentVotes(string(account.Actor), api)
	case fieldType == "transaction":
		returnValue = `{
  "context_free_actions": [],
  "actions": [
    {
      "signatures": [
        "SIG_K1_..."
      ],
      "compression": "none",
      "packed_context_free_data": "",
      "packed_trx": "b474345e54..."
    }
  ],
  "transaction_extensions": []
}`
	case fieldType == "asset":
		returnValue = "100000.000000000 FIO"
	case (fieldName == "to" || fieldName == "from") && fieldType == "name":
		returnValue = string(account.Actor)
	case fieldType == "permission_level":
		returnValue = fmt.Sprintf(`{
    "actor":"%s",
    "permission":"active"
}`, account.Actor)
	case fieldName == "periods":
		returnValue = `[
    {
        "duration": 86400,
        "percent": 1.2
    },
    {
        "duration": 172800,
        "percent": 90.8
    },
    {
        "duration": 259200,
        "percent": 8.0
    }
]`
	case fieldType == "permission_level[]":
		returnValue = fmt.Sprintf(`[{
        "actor":"%s",
        "permission":"active"
}]`, account.Actor)
	case fieldType == "proposal":
		returnValue = `{
    "proposal_name":"proposal",
    "packed_transaction":"0x0a0b0c0d0e0f"
}`
	case fieldType == "blockchain_parameters":
		returnValue = `{
    "max_block_net_usage": 1048576,
    "target_block_net_usage_pct": 1000,
    "max_transaction_net_usage": 524288,
    "base_per_transaction_net_usage": 12,
    "net_usage_leeway": 500,
    "context_free_discount_net_usage_num": 20,
    "context_free_discount_net_usage_den": 100,
    "max_block_cpu_usage": 200000,
    "target_block_cpu_usage_pct": 1000,
    "max_transaction_cpu_usage": 150000,
    "min_transaction_cpu_usage": 100,
    "max_transaction_lifetime": 3600,
    "deferred_trx_expiration_window": 600,
    "max_transaction_delay": 3888000,
    "max_inline_action_size": 4096,
    "max_inline_action_depth": 4,
    "max_authority_depth": 6,
    "last_producer_schedule_update": 1580492400,
    "last_pervote_bucket_fill": 1580492400,
    "pervote_bucket": 0,
    "perblock_bucket": 0,
    "total_unpaid_blocks": 0,
    "total_voted_fio": 76103319000000000,
    "thresh_voted_fio_time": 1580492400,
    "last_producer_schedule_size": 3,
    "total_producer_vote_weight": "75000307600000000.00000000000000000",
    "last_name_close": 1580492400,
    "last_fee_update": 1580492400
}`
	case fieldType == "block_header":
		returnValue = `{
  "timestamp": 1580492400,
  "producer": "eosio",
  "confirmed": 1,
  "previous": "0000000000000000000000000000000000000000000000000000000000000000",
  "transaction_mroot": "0000000000000000000000000000000000000000000000000000000000000000",
  "action_mroot": "0000000000000000000000000000000000000000000000000000000000000000",
  "schedule_version": 0
}`
	case fieldName == "end_point":
		returnValue = `auth_update`
	case fieldType == "extension":
		returnValue = `{"type": 1, "data": "0x0a0b0c0d0e0f"}`
	case fieldType == "feevalue":
		returnValue = `{
    "end_point": "register_fio_domain",
    "value": 8000000000
}`
	case fieldType == "feevalue[]":
		returnValue = `[{
    "end_point": "register_fio_domain",
    "value": 8000000000
  },
  {
    "end_point": "register_fio_address",
    "value": 1000000000
  }
]`
	case strings.HasPrefix(fieldType, "checksum256"):
		rc := make([]byte, 32)
		rand.Read(rc)
		returnValue = hex.EncodeToString(rc)
	default:
		returnValue = ""
	}
	return returnValue
}

func getInterface(t string) interface{} {
	types := map[string]interface{}{
		"authority": eos.Authority{},
		"bool":      eos.Bool(true),
		"byte":      byte(0),
		//"byte[]": make([]byte,0),
		"byte[]":       "",
		"checksum256":  eos.Checksum256{},
		"checksum256$": eos.Checksum256{},
		"float128":     eos.Float128{},
		"float32":      float32(0),
		"float64":      float64(0),
		"hex_bytes":    eos.HexBytes{},
		"int128":       eos.Int128{},
		"int16":        int16(0),
		"int32":        int32(0),
		//"int64": eos.Int64(0),
		"int64":      int64(0),
		"int8":       int8(0),
		"name":       "",
		"public_key": "",
		"signature":  eos.HexBytes{},
		"string":     "",
		"symbol":     eos.Symbol{},
		"time":       eos.Tstamp{},
		"timestamp":  eos.Tstamp{},
		"uint128":    eos.Uint128{},
		"uint16":     uint16(0),
		"uint32":     uint32(0),
		"uint64":     eos.Uint64(0),
		"varint32":   eos.Varint32(0),
		"varuint32":  eos.Varuint32(0),
	}
	if types[t] != nil {
		return types[t]
	}
	return ""
}

type abiField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type abiStruct struct {
	Name   string     `json:"name"`
	Base   string     `json:"base"`
	Fields []abiField `json:"fields"`
}

func (abi *Abi) DeriveJsonAbi() (abiJson []byte) {
	a := abiStruct{
		Name:   abi.Action,
		Fields: make([]abiField, 0),
	}
	for _, field := range abi.Rows {
		t := field.Type.Selected
		if field.typeOverride != "" {
			t = field.typeOverride
		}
		a.Fields = append(a.Fields, abiField{
			Name: *field.Name,
			Type: t,
		},
		)
	}
	j, e := json.MarshalIndent(a, "", "  ")
	if e != nil {
		errs.ErrChan <- "could not generate ABI json: " + e.Error()
		return nil
	}
	return j
}

var PrivilegedActions = map[string]bool{
	"eosio::addaction":           true,
	"eosio::addlocked":           true,
	"eosio::burnaction":          true,
	"eosio::canceldelay":         true,
	"eosio::crautoproxy":         true,
	"eosio::incram":              true,
	"eosio::inhibitunlck":        true,
	"eosio::init":                true,
	"eosio::setpriv":             true,
	"eosio::newaccount":          true,
	"eosio::onblock":             true,
	"eosio::onerror":             true,
	"eosio::resetclaim":          true,
	"eosio::remaction":           true,
	"eosio::rmvproducer":         true,
	"eosio::setabi":              true,
	"eosio::setautoproxy":        true,
	"eosio::setcode":             true,
	"eosio::setparams":           true,
	"eosio::unlocktokens":        true,
	"eosio::updatepower":         true,
	"eosio::updlbpclaim":         true,
	"eosio::updlocked":           true,
	"eosio::updtrevision":        true,
	"eosio.wrap::execute":        true,
	"fio.address::bind2eosio":    true,
	"fio.address::decrcounter":   true,
	"fio.token::create":          true,
	"fio.token::issue":           true,
	"fio.token::mintfio":         true,
	"fio.token::retire":          true,
	"fio.token::transfer":        true,
	"fio.tpid::rewardspaid":      true,
	"fio.tpid::updatebounty":     true,
	"fio.tpid::updatetpid":       true,
	"fio.treasury::startclock":   true,
	"fio.treasury::bppoolupdate": true,
	"fio.treasury::bprewdupdate": true,
	"fio.treasury::fdtnrwdupdat": true,
}

var ProducerActions = map[string]bool{
	"fio.treasury::bpclaim":    true,
	"fio.address::burnexpired": true,
	"fio.fee::bundlevote":      true,
	"fio.fee::bytemandfee":     true,
	"fio.fee::createfee":       true,
	"fio.fee::mandatoryfee":    true,
	"fio.fee::setfeemult":      true,
	"fio.fee::setfeevote":      true,
	"fio.fee::updatefees":      true,
	"eosio::regproducer":       true,
	"eosio::unregprod":         true,
}
