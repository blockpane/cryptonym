package cryptonym

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type AccountInformation struct {
	*sync.Mutex

	Actor      string   `json:"actor"`
	PubKey     string   `json:"pub_key"`
	PrivKey    string   `json:"priv_key"`
	Balance    int64    `json:"balance"`
	BundleCred int      `json:"bundle_cred"`
	MsigOwners []string `json:"msig_owners"`
	MsigThresh uint32   `json:"msig_thresh"`
	RamUsed    int64    `json:"ram_used"`
	fioNames   []string
	FioNames   []FioAddressStruct `json:"fio_names"`
	fioDomains []string
	FioDomains []FioDomainStruct `json:"fio_domains"`
	PublicKeys []AddressesList   `json:"public_keys"`
	api        *fio.API
	Producer   *ProducerInfo `json:"producer"`
}

type FioAddressStruct struct {
	Id           int             `json:"id"`
	Name         string          `json:"name"`
	NameHash     string          `json:"namehash"`
	Domain       string          `json:"domain"`
	DomainHash   string          `json:"domainhash"`
	Expiration   int64           `json:"expiration"`
	OwnerAccount string          `json:"owner_account"`
	Addresses    []AddressesList `json:"addresses"`
	BundleCount  uint64          `json:"bundleeligiblecountdown"`
}

type FioDomainStruct struct {
	Name       string          `json:"name"`
	IsPublic   uint8           `json:"is_public"`
	Expiration int64           `json:"expiration"`
	Account    eos.AccountName `json:"account"`
}

type AddressesList struct {
	TokenCode     string `json:"token_code"`
	ChainCode     string `json:"chain_code"`
	PublicAddress string `json:"public_address"`
}

type ProducerInfo struct {
	Owner             string    `json:"owner"`
	FioAddress        string    `json:"fio_address"`
	TotalVotes        float64   `json:"total_votes"`
	ProducerPublicKey string    `json:"producer_public_key"`
	IsActive          bool      `json:"is_active"`
	Url               string    `json:"url"`
	UnpaidBlocks      int       `json:"unpaid_blocks"`
	LastClaimTime     time.Time `json:"last_claim_time"`
	Location          int       `json:"location"`
}

var bpLocationMux sync.RWMutex
var bpLocationMap = map[int]string{
	10: "East Asia",
	20: "Australia",
	30: "West Asia",
	40: "Africa",
	50: "Europe",
	60: "East North America",
	70: "South America",
	80: "West North America",
}

var accountSearchType = []string{
	"Public Key",
	"Fio Address",
	"Private Key",
	"Actor/Account",
	"Fio Domain", // TODO: how is index derived on fio.address domains table?
}

func GetLocation(i int) string {
	bpLocationMux.RLock()
	defer bpLocationMux.RUnlock()
	loc := bpLocationMap[i]
	if loc == "" {
		return "Invalid Location"
	}
	return loc
}

func AccountSearch(searchFor string, value string) (as *AccountInformation, err error) {
	as = &AccountInformation{}
	as.api, _, err = fio.NewConnection(nil, Uri)
	if err != nil {
		return nil, err
	}
	as.api.Header.Set("User-Agent", "fio-cryptonym-wallet")
	switch searchFor {
	case "Actor/Account":
		return as, as.searchForActor(value)
	case "Public Key":
		return as, as.searchForPub(value)
	case "Private Key":
		return as, as.searchForPriv(value)
	case "Fio Address":
		return as, as.searchForAddr(value)
	case "Fio Domain":
		return as, as.searchForDom(value)
	}
	return nil, nil
}

type aMap struct {
	Clientkey string `json:"clientkey"`
}

func (as *AccountInformation) searchForActor(s string) error {
	if s == "eosio" || strings.HasPrefix(s, "eosio.") || strings.HasPrefix(s, "fio.") {
		resp, err := as.api.GetFioAccount(s)
		if err != nil {
			return err
		}
		as.PubKey = "n/a"
		as.Actor = s
		if len(resp.Permissions) > 0 {
			if len(resp.Permissions[0].RequiredAuth.Keys) == 1 {
				as.PubKey = resp.Permissions[0].RequiredAuth.Keys[0].PublicKey.String()
			}
			for _, p := range resp.Permissions {
				if len(p.RequiredAuth.Accounts) > 0 {
					for _, a := range p.RequiredAuth.Accounts {
						as.MsigOwners = append(as.MsigOwners, string(a.Permission.Actor))
					}
				}
			}
		}
		if as.PubKey != "n/a" {
			as.PubKey = "Warning, not msig! - " + as.PubKey
		}
		return nil
	}
	name, err := eos.StringToName(s)
	if err != nil {
		return err
	}
	resp, err := as.api.GetTableRows(eos.GetTableRowsRequest{
		Code:       "fio.address",
		Scope:      "fio.address",
		Table:      "accountmap",
		LowerBound: fmt.Sprintf("%d", name),
		UpperBound: fmt.Sprintf("%d", name),
		Limit:      math.MaxInt32,
		KeyType:    "i64",
		Index:      "1",
		JSON:       true,
	})
	if err != nil {
		fmt.Println(err)
	}
	found := make([]aMap, 0)
	err = json.Unmarshal(resp.Rows, &found)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return errors.New("no matching account found in fio.address accountmap table")
	}
	as.Actor = s
	as.PubKey = found[0].Clientkey
	return as.searchForPub(as.PubKey)
}

func (as *AccountInformation) searchForPub(s string) error {
	names, found, err := as.api.GetFioNames(s)
	if err != nil {
		return err
	}
	as.PubKey = s
	a, err := fio.ActorFromPub(s)
	if err != nil {
		return err
	}
	assets, err := as.api.GetCurrencyBalance(a, "FIO", "fio.token")
	if err != nil {
		return err
	}
	if len(assets) > 0 {
		as.Balance = int64(assets[0].Amount)
	}
	as.Actor = string(a)
	if found {
		for _, n := range names.FioAddresses {
			as.fioNames = appendUniq(as.fioNames, n.FioAddress)
		}
		for _, n := range names.FioDomains {
			as.fioDomains = appendUniq(as.fioDomains, n.FioDomain)
		}
	}
	as.getFioNames()
	as.getFioDomains()
	as.getExtra()
	return nil
}

func (as *AccountInformation) searchForPriv(s string) error {
	a, err := fio.NewAccountFromWif(s)
	if err != nil {
		return err
	}
	as.PrivKey = s
	as.PubKey = a.PubKey
	return as.searchForPub(a.PubKey)
}

func (as *AccountInformation) searchForAddr(s string) error {
	pubAddr, found, err := as.api.PubAddressLookup(fio.Address(s), "FIO", "FIO")
	if err != nil {
		return err
	}
	if !found {
		return errors.New("did not find any FIO public keys for that address")
	}
	as.fioNames = appendUniq(as.fioNames, s)
	as.PubKey = pubAddr.PublicAddress
	return as.searchForPub(pubAddr.PublicAddress)
}

func (as *AccountInformation) getFioNames() {
	const limit = 20
	n, err := eos.StringToName(as.Actor)
	if err != nil {
		errs.ErrChan <- err.Error()
		return
	}
	name := fmt.Sprintf("%d", n)
	row, err := as.api.GetTableRows(eos.GetTableRowsRequest{
		Code:       "fio.address",
		Scope:      "fio.address",
		Table:      "fionames",
		LowerBound: name,
		UpperBound: name,
		Limit:      limit,
		KeyType:    "i64",
		Index:      "4",
		JSON:       true,
	})
	if err != nil {
		errs.ErrChan <- err.Error()
		return
	}
	if len(row.Rows) > 2 {
		fNames := make([]FioAddressStruct, 0)
		err = json.Unmarshal(row.Rows, &fNames)
		if err != nil {
			errs.ErrChan <- err.Error()
			return
		}
		as.FioNames = append(as.FioNames, fNames...)
	}
	if row.More {
		errs.ErrChan <- fmt.Sprintf("truncated results to first %d addresses", limit)
	}
}

func (as *AccountInformation) getFioDomains() {
	const limit = 20
	row, err := as.api.GetTableRows(eos.GetTableRowsRequest{
		Code:       "fio.address",
		Scope:      "fio.address",
		Table:      "domains",
		LowerBound: as.Actor,
		UpperBound: as.Actor,
		Limit:      limit,
		KeyType:    "name",
		Index:      "2",
		JSON:       true,
	})
	if err != nil {
		errs.ErrChan <- err.Error()
		return
	}
	if len(row.Rows) > 2 {
		fDoms := make([]FioDomainStruct, 0)
		err = json.Unmarshal(row.Rows, &fDoms)
		if err != nil {
			errs.ErrChan <- err.Error()
			return
		}
	doms:
		for _, fDom := range fDoms {
			for _, existing := range as.FioDomains {
				if existing.Name == fDom.Name {
					continue doms
				}
			}
			as.FioDomains = append(as.FioDomains, fDom)
		}
	}
	if row.More {
		errs.ErrChan <- fmt.Sprintf("truncated results to first %d domains", limit)
	}
}

// only works on >= v0.9.0
func (as *AccountInformation) searchForDom(s string) error {
	ss := FioDomainNameHash(s)
	resp, err := as.api.GetTableRows(eos.GetTableRowsRequest{
		Code:       "fio.address",
		Scope:      "fio.address",
		Table:      "domains",
		LowerBound: ss,
		UpperBound: ss,
		Limit:      1,
		KeyType:    "i128",
		Index:      "4",
		JSON:       true,
	})
	if err != nil {
		return err
	}
	if len(resp.Rows) > 2 {
		d := make([]FioDomainStruct, 0)
		err = json.Unmarshal(resp.Rows, &d)
		if err != nil {
			return err
		}
		as.FioDomains = append(as.FioDomains, d...)
		if as.Actor == "" && len(d) > 0 && d[0].Account != "" {
			return as.searchForActor(string(d[0].Account))
		}
	}
	return nil
}

func (as *AccountInformation) getExtra() {
	if as.Actor != "" {
		acc, err := as.api.GetFioAccount(as.Actor)
		if err != nil {
			return
		}
		as.RamUsed = int64(acc.RAMUsage)
		for _, a := range acc.Permissions {
			if a.PermName == "active" && a.RequiredAuth.Accounts != nil && len(a.RequiredAuth.Accounts) > 0 {
				as.MsigThresh = a.RequiredAuth.Threshold
				for _, owner := range a.RequiredAuth.Accounts {
					as.MsigOwners = append(as.MsigOwners, fmt.Sprintf("%s (weight: %d)", owner.Permission.Actor, owner.Weight))
				}
			}
		}
	}
}

func FioDomainNameHash(s string) string {
	sha := sha1.New()
	sha.Write([]byte(s))
	// last 16 bytes of sha1-sum, as big-endian
	return "0x" + hex.EncodeToString(FlipEndian(sha.Sum(nil)))[8:]
}

func FlipEndian(orig []byte) []byte {
	flipped := make([]byte, len(orig))
	for i := range orig {
		flipped[len(flipped)-i-1] = orig[i]
	}
	return flipped
}

func appendUniq(oldSlice []string, add ...string) (newSlice []string) {
	u := make(map[string]bool)
	oldSlice = append(oldSlice, add...)
	for _, v := range oldSlice {
		u[v] = true
	}
	for k := range u {
		newSlice = append(newSlice, k)
	}
	sort.Strings(newSlice)
	return
}
