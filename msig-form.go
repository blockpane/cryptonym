package cryptonym

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	fioassets "github.com/blockpane/cryptonym/assets"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type signer struct {
	weight *widget.Entry
	actor  *widget.Entry
	index  int
}

var (
	MsigRefreshRequests = make(chan bool)
	MsigLastTab         = 0
	MsigLoaded          bool
)

func UpdateAuthContent(container chan fyne.Container, api *fio.API, opts *fio.TxOptions, account *fio.Account) {
	for !Connected {
		time.Sleep(time.Second)
	}
	authTab := func() {} //recursive
	authTab = func() {
		accountEntry := widget.NewEntry()
		newAccount := &fio.Account{}
		update := &widget.TabItem{}
		fee := widget.NewLabelWithStyle(p.Sprintf("Required Fee: %s %G", fio.FioSymbol, fio.GetMaxFee(fio.FeeAuthUpdate)*2.0), fyne.TextAlignTrailing, fyne.TextStyle{})
		warning := widget.NewHBox(
			widget.NewIcon(theme.WarningIcon()),
			widget.NewLabelWithStyle("Warning: converting active account to multi-sig!", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		)
		warning.Hide()
		newRandCheck := widget.NewCheck("Create New and Burn", func(b bool) {
			if b {
				newAccount, _ = fio.NewRandomAccount()
				accountEntry.SetText(string(newAccount.Actor))
				fee.SetText(p.Sprintf("Required Fee: %s%G", fio.FioSymbol, fio.GetMaxFee(fio.FeeAuthUpdate)*2.0+fio.GetMaxFee(fio.FeeTransferTokensPubKey)))
				fee.Refresh()
				warning.Hide()
			} else {
				accountEntry.SetText(string(account.Actor))
				fee.SetText(p.Sprintf("Required Fee: %s%G", fio.FioSymbol, fio.GetMaxFee(fio.FeeAuthUpdate)*2.0))
				fee.Refresh()
				warning.Show()
			}
		})
		accountEntry.SetText(string(account.Actor))
		newRandCheck.SetChecked(true)

		threshEntry := widget.NewEntry()
		threshEntry.SetText("2")
		tMux := sync.Mutex{}
		threshEntry.OnChanged = func(s string) {
			tMux.Lock()
			time.Sleep(300 * time.Millisecond)
			if _, e := strconv.Atoi(s); e != nil {
				tMux.Unlock()
				threshEntry.SetText("2")
				return
			}
			tMux.Unlock()
		}

		signerSlice := make([]signer, 0) // keeps order correct when adding rows, and is sorted when submitting tx
		newSigner := func(s string) *fyne.Container {
			if s == "" {
				for i := 0; i < 12; i++ {
					b := []byte{uint8(len(signerSlice) + 96)} // assuming we will start with 1 by default
					s = s + string(b)
				}
			}
			w := widget.NewEntry()
			w.SetText("1")
			a := widget.NewEntry()
			a.SetText(s)
			index := len(signerSlice)
			shouldAppend := func() bool {
				for _, sc := range signerSlice {
					if sc.actor == a {
						return false
					}
				}
				return true
			}
			if shouldAppend() {
				signerSlice = append(signerSlice, signer{
					weight: w,
					actor:  a,
					index:  index,
				})
			} else {
				return nil
			}
			threshEntry.SetText(fmt.Sprintf("%d", 1+len(signerSlice)/2))
			threshEntry.Refresh()
			return fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(6),
				layout.NewSpacer(),
				widget.NewLabelWithStyle("Actor "+strconv.Itoa(signerSlice[index].index+1)+": ", fyne.TextAlignTrailing, fyne.TextStyle{}),
				signerSlice[index].actor,
				widget.NewLabelWithStyle("Vote Weight: ", fyne.TextAlignTrailing, fyne.TextStyle{}),
				signerSlice[index].weight,
				layout.NewSpacer(),
			)
		}
		signerGroup := widget.NewGroup(" Signers ", newSigner(string(account.Actor)))
		addSigner := widget.NewButtonWithIcon("Add Signer", theme.ContentAddIcon(), func() {
			signerGroup.Append(newSigner(""))
			signerGroup.Refresh()
			update.Content.Refresh()
		})

		resetSigners := widget.NewButtonWithIcon("Reset", theme.ContentClearIcon(), func() {
			MsigLastTab = 1
			authTab()
			go func() {
				time.Sleep(100 * time.Millisecond)
				for _, a := range fyne.CurrentApp().Driver().AllWindows() {
					a.Content().Refresh()
				}
			}()
		})

		submitButton := &widget.Button{}
		submitButton = widget.NewButtonWithIcon("Submit", fioassets.NewFioLogoResource(), func() {
			submitButton.Disable()
			ok, _, msg := checkSigners(signerSlice, "active")
			if !ok {
				dialog.ShowError(msg, Win)
				return
			}
			defer submitButton.Enable()
			if newRandCheck.Checked {
				if ok, err := fundRandMsig(newAccount, account, len(signerSlice), api, opts); !ok {
					errs.ErrChan <- "new msig account was not created!"
					dialog.ShowError(err, Win)
					return
				}
			}
			acc := &fio.Account{}
			acc = account
			if newRandCheck.Checked {
				acc = newAccount
			}
			t, err := strconv.Atoi(threshEntry.Text)
			if err != nil {
				errs.ErrChan <- "Invalid threshold, refusing to continue"
				return
			}
			ok, info, err := updateAuthResult(acc, signerSlice, t)
			if ok {
				dialog.ShowCustom("Success", "OK", info, Win)
				return
			}
			dialog.ShowError(err, Win)
		})

		update = widget.NewTabItem("Update Auth",
			widget.NewScrollContainer(
				widget.NewVBox(
					widget.NewHBox(
						widget.NewLabelWithStyle("Account: ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						newRandCheck,
						accountEntry,
						widget.NewLabelWithStyle("Threshold: ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						threshEntry,
					),
					signerGroup,
					layout.NewSpacer(),
					widget.NewHBox(layout.NewSpacer(), addSigner, resetSigners, layout.NewSpacer(), warning, fee, submitButton, layout.NewSpacer()),
					widget.NewLabel(""),
				),
			))
		tabs := widget.NewTabContainer(MsigRequestsContent(api, opts, account), update)
		tabs.SelectTabIndex(MsigLastTab)
		container <- *fyne.NewContainerWithLayout(layout.NewMaxLayout(), tabs)
	}
	go func() {
		for {
			select {
			case r := <-MsigRefreshRequests:
				if r {
					a := *Api
					api = &a
					o := *Opts
					opts = &o
					u := *Account
					account = &u
					authTab()
				}
				// do we ever need a new routine? Think this return is causing a deadlock...
				//return
			}
		}
	}()
	authTab()
	MsigLoaded = true
}

func getSigners(account string, api *fio.API) string {
	if strings.HasPrefix(account, "eosio") {
		return getTopProds(api)
	}
	info, err := api.GetFioAccount(account)
	if err != nil {
		return ""
	}
	for _, auth := range info.Permissions {
		if auth.PermName == "active" && auth.RequiredAuth.Accounts != nil && len(auth.RequiredAuth.Accounts) > 0 {
			signers := make([]string, 0)
			for _, signer := range auth.RequiredAuth.Accounts {
				signers = append(signers, string(signer.Permission.Actor))
			}
			if len(signers) != 0 {
				return strings.Join(signers, ", ")
			}
		}
	}
	return account + " is not a multi-sig account!"
}

type topProd struct {
	Producer string `json:"owner"`
	IsActive uint8  `json:"is_active"`
}

func getTopProds(api *fio.API) string {
	const want = 30
	gtr, err := api.GetTableRowsOrder(fio.GetTableRowsOrderRequest{
		Code:       "eosio",
		Scope:      "eosio",
		Table:      "producers",
		LowerBound: "0",
		UpperBound: "18446744073709551615",
		Index:      "4",
		KeyType:    "i64",
		Limit:      100,
		JSON:       true,
		Reverse:    true,
	})
	if err != nil {
		errs.ErrChan <- err.Error()
		return ""
	}
	tp := make([]topProd, 0)
	err = json.Unmarshal(gtr.Rows, &tp)
	if err != nil {
		errs.ErrChan <- err.Error()
		return ""
	}
	prods := make([]string, 0)
	for _, p := range tp {
		if p.IsActive == 1 {
			prods = append(prods, p.Producer)
			if len(prods) == want {
				break
			}
		}
	}
	sort.Strings(prods)
	return strings.Join(prods, ", ")
}

func fundRandMsig(msig *fio.Account, funder *fio.Account, count int, api *fio.API, opts *fio.TxOptions) (ok bool, err error) {
	feeMultGuess := float64(42*count) / 1000.0
	feeMultGuess = math.Ceil(feeMultGuess)
	if feeMultGuess < 1.0 {
		feeMultGuess = 1
	}
	if feeMultGuess > 1.0 {
		errs.ErrChan <- fmt.Sprintf("NOTE: fees are increased due to number of signers, transaction will be %s %g", fio.FioSymbol, fio.GetMaxFee(fio.FeeAuthUpdate)*feeMultGuess*2)
	}
	errs.ErrChan <- "creating new msig account, sending funds, please wait"
	resp, err := api.SignPushTransaction(
		fio.NewTransaction(
			[]*fio.Action{fio.NewTransferTokensPubKey(funder.Actor, msig.PubKey, fio.Tokens((fio.GetMaxFee(fio.FeeAuthUpdate)*feeMultGuess*2.0)+fio.GetMaxFee(fio.FeeTransferTokensPubKey)))},
			opts,
		), opts.ChainID, fio.CompressionNone,
	)
	if err != nil {
		errs.ErrChan <- err.Error()
		errs.ErrChan <- "Could not fund new msig account:"
		return false, err
	}
	errs.ErrChan <- p.Sprintf("Funded new account (%s) txid: %v", resp.TransactionID)
	BalanceChan <- true
	return true, nil
}

func checkSigners(signers []signer, level eos.PermissionName) (ok bool, permLevelWeight []eos.PermissionLevelWeight, msg error) {
	weights := make([]eos.PermissionLevelWeight, 0)
	signerWeights := make(map[string]int)
	unique := make(map[string]bool)
	signerOrder := make([]string, 0)
	for _, s := range signers {

		def := bytes.Repeat([]byte(s.actor.Text[:1]), 12)
		if s.actor.Text == string(def) {
			// reject on a default value
			msg = errors.New(s.actor.Text + " is not a valid signer for an msig account, refusing to continue.")
			errs.ErrChan <- msg.Error()
			return false, nil, msg
		}
		signerOrder = append(signerOrder, s.actor.Text)
		w, e := strconv.Atoi(s.weight.Text)
		if e != nil {
			msg = errors.New("invalid weight specified for actor: " + s.actor.Text)
			errs.ErrChan <- msg.Error()
			return false, nil, msg
		}
		signerOrder = append(signerOrder, s.actor.Text)
		signerWeights[s.actor.Text] = w
	}
	sort.Strings(signerOrder)
	for _, so := range signerOrder {
		if unique[so] {
			continue
		}
		unique[so] = true
		weights = append(weights, eos.PermissionLevelWeight{
			Permission: eos.PermissionLevel{
				Actor:      eos.AccountName(so),
				Permission: level,
			},
			Weight: uint16(signerWeights[so]),
		})
	}
	if len(weights) > 0 {
		return true, weights, nil
	}
	return false, nil, errors.New("could not parse permission levels")
}

// getRequestsForAccount will query the propose table, and returns a map with the proposal name, and a zlib compressed copy
// of the packed transactions (they can be huge!)
func getRequestsForAccount(actor string, limit int, lowerLimit string, account *fio.Account, api *fio.API, opts *fio.TxOptions) (map[string][]byte, error) {
	// TODO: all the queries.
	return nil, nil
}

func updateAuthResult(account *fio.Account, signers []signer, threshold int) (ok bool, result *widget.Box, err error) {
	ok, activePermLevel, _ := checkSigners(signers, "active")
	ok, ownerPermLevel, _ := checkSigners(signers, "owner")
	if !ok {
		return ok, nil, errors.New("update auth Failed, an invalid actor was supplied")
	}
	a, o, e := fio.NewConnection(account.KeyBag, Uri)
	if e != nil {
		errs.ErrChan <- e.Error()
		errs.ErrChan <- "Could not update auth, new connection failed:"
		return false, nil, errors.New("update auth Failed, could not connect to server")
	}
	a.Header.Set("User-Agent", "fio-cryptonym-wallet")
	feeMultGuess := float64(42*len(activePermLevel)) / 1000.0
	feeMultGuess = math.Ceil(feeMultGuess)
	if feeMultGuess < 1 {
		feeMultGuess = 1.0
	}
	updateActive := fio.NewAction("eosio", "updateauth", account.Actor, fio.UpdateAuth{
		Account:    account.Actor,
		Permission: "active",
		Parent:     "owner",
		Auth: fio.Authority{
			Threshold: uint32(threshold),
			Accounts:  activePermLevel,
		},
		MaxFee: fio.Tokens(fio.GetMaxFee(fio.FeeAuthUpdate) * feeMultGuess),
	})
	buf := bytes.NewBuffer(nil)
	updateOwner := fio.NewActionWithPermission("eosio", "updateauth", account.Actor, "owner", fio.UpdateAuth{
		Account:    account.Actor,
		Permission: "owner",
		//Parent:     "owner",
		Auth: fio.Authority{
			Threshold: uint32(threshold),
			Accounts:  ownerPermLevel,
		},
		MaxFee: fio.Tokens(fio.GetMaxFee(fio.FeeAuthUpdate) * feeMultGuess),
	})
	_, tx, e := a.SignTransaction(
		fio.NewTransaction(
			[]*fio.Action{updateOwner, updateActive}, o),
		o.ChainID, fio.CompressionNone,
	)
	if e != nil {
		errs.ErrChan <- e.Error()
		errs.ErrChan <- account.KeyBag.Keys[0].String()
		errs.ErrChan <- "use this private key to recover funds."
		errs.ErrChan <- "Could not update auth for owner, sign transaction failed:"
		return false, nil, errors.New("Update Auth Failed: " + e.Error())
	}
	out, e := a.PushTransactionRaw(tx)
	if e != nil {
		errs.ErrChan <- e.Error()
		errs.ErrChan <- account.KeyBag.Keys[0].String()
		errs.ErrChan <- "use this private key to recover funds."
		errs.ErrChan <- "Could not update auth for owner, push transaction failed:"
		return false, nil, errors.New("Update Auth Failed: " + e.Error())
	}
	j, _ := json.MarshalIndent(out, "", "    ")
	if len(j) > 2 {
		buf.Write(j)
	}
	actors := make([]string, 0)
	for _, act := range signers {
		actors = append(actors, act.actor.Text)
	}
	sort.Strings(actors)
	txResult := widget.NewMultiLineEntry()
	txResult.SetText(string(buf.Bytes()))
	errs.ErrChan <- fmt.Sprintf("Update auth success: %s updated permsissions with signers %v", string(account.Actor), actors)
	return true, widget.NewVBox(
		widget.NewHBox(
			widget.NewLabel("Successfully created new msig account for "+string(account.Actor)+" "),
			widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
				cb := Win.Clipboard()
				cb.SetContent(string(account.Actor))
			}),
		),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*50)/100, (H*50)/100)),
			widget.NewScrollContainer(
				txResult,
			),
		),
	), nil
}
