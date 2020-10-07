package cryptonym

import (
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/canvas"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"sort"
	"strconv"
	"sync"
	"time"
)

const spaces = "                                                     " // placeholder for pubkeys

var RefreshRequestsChan = make(chan bool)

func RequestContent(reqContent chan fyne.CanvasObject, refresh chan bool) {
	content, err := GetPending(refresh, Account, Api)
	if err != nil {
		panic(err)
	}
	reqContent <- content
	go func() {
		for {
			select {
			case <-refresh:
				content, err := GetPending(refresh, Account, Api)
				if err != nil {
					errs.ErrChan <- err.Error()
					continue
				}
				reqContent <- content
			}
		}
	}()
}

func GetPending(refreshChan chan bool, account *fio.Account, api *fio.API) (form fyne.CanvasObject, err error) {
	sendNew := widget.NewButtonWithIcon("Request Funds", theme.DocumentCreateIcon(), func() {
		closed := make(chan interface{})
		d := dialog.NewCustom(
			"Send a new funds request",
			"Cancel",
			NewRequest(account, api),
			Win,
		)
		go func() {
			<-closed
			d.Hide()
		}()
		d.SetOnClosed(func() {
			refreshChan <- true
		})
		d.Show()
	})
	refr := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		refreshChan <- true
	})
	topDesc := widget.NewLabel("")
	top := widget.NewHBox(
		layout.NewSpacer(),
		topDesc,
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(refr.MinSize()), refr),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(sendNew.MinSize()), sendNew),
		layout.NewSpacer(),
	)

	pending, has, err := api.GetPendingFioRequests(account.PubKey, 101, 0)
	if err != nil {
		return widget.NewHBox(widget.NewLabel(err.Error())), err
	}
	if !has {
		return widget.NewVBox(top, widget.NewLabel("No pending requests.")), err
	}
	howMany := len(pending.Requests)
	topDesc.SetText(fmt.Sprint(howMany) + " pending requests.")
	if howMany > 100 {
		topDesc.SetText("More than 100 pending requests.")
	}
	sort.Slice(pending.Requests, func(i, j int) bool {
		return pending.Requests[i].FioRequestId < pending.Requests[j].FioRequestId
	})
	if howMany > 25 {
		topDesc.SetText(topDesc.Text + fmt.Sprintf(" (only first 25 displayed.)"))
		pending.Requests = pending.Requests[:25]
	}

	requests := fyne.NewContainerWithLayout(layout.NewGridLayout(5),
		widget.NewLabelWithStyle("Actions", fyne.TextAlignLeading, fyne.TextStyle{}),
		widget.NewLabelWithStyle("ID / Time", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("From", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("To", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Summary", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	)

	for _, req := range pending.Requests {
		func(req fio.RequestStatus) {
			id := widget.NewLabelWithStyle(fmt.Sprintf("%d | "+req.TimeStamp.Local().Format(time.Stamp), req.FioRequestId), fyne.TextAlignCenter, fyne.TextStyle{})
			payer := req.PayerFioAddress
			if len(payer) > 32 {
				payer = payer[:29] + "..."
			}
			payee := req.PayeeFioAddress
			if len(payee) > 32 {
				payee = payee[:29] + "..."
			}
			fr := widget.NewLabelWithStyle(payee, fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
			to := widget.NewLabelWithStyle(payer, fyne.TextAlignLeading, fyne.TextStyle{})
			view := widget.NewButtonWithIcon("View", theme.VisibilityIcon(), func() {
				closed := make(chan interface{})
				d := dialog.NewCustom(
					fmt.Sprintf("FIO Request ID %d (%s)", req.FioRequestId, req.PayeeFioAddress),
					"Close",
					ViewRequest(req.FioRequestId, closed, refreshChan, account, api),
					Win,
				)
				go func() {
					<-closed
					d.Hide()
					refreshChan <- true
				}()
				d.Show()
			})
			rejectBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				_, err := api.SignPushActions(fio.NewRejectFndReq(account.Actor, strconv.FormatUint(req.FioRequestId, 10)))
				if err != nil {
					errs.ErrChan <- err.Error()
					return
				}
				errs.ErrChan <- "rejected request id: " + strconv.FormatUint(req.FioRequestId, 10)
				refreshChan <- true
			})
			rejectBtn.HideShadow = true
			requests.AddObject(widget.NewHBox(view, layout.NewSpacer()))
			requests.AddObject(id)
			requests.AddObject(fr)
			requests.AddObject(to)
			obt, err := fio.DecryptContent(account, req.PayeeFioPublicKey, req.Content, fio.ObtRequestType)
			var summary string
			if err != nil {
				view.Hide()
				summary = "invalid content"
				errs.ErrChan <- err.Error()
			} else {
				summary = obt.Request.ChainCode
				if obt.Request.ChainCode != obt.Request.TokenCode {
					summary += "/" + obt.Request.TokenCode
				}
				summary += fmt.Sprintf(" (%s) %q", obt.Request.Amount, obt.Request.Memo)
				if len(summary) > 32 {
					summary = summary[:29] + "..."
				}
			}
			requests.AddObject(widget.NewHBox(layout.NewSpacer(), widget.NewLabelWithStyle(summary, fyne.TextAlignTrailing, fyne.TextStyle{Italic: true}), rejectBtn))
		}(req)
	}
	form = widget.NewVBox(
		top,
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), int(float32(PctHeight())*.68))),
			widget.NewScrollContainer(widget.NewVBox(requests,
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{
					Width:  20,
					Height: 50,
				}), layout.NewSpacer()),
			)),
		),
	)
	return
}

func ViewRequest(id uint64, closed chan interface{}, refresh chan bool, account *fio.Account, api *fio.API) fyne.CanvasObject {
	req, err := api.GetFioRequest(id)
	if err != nil {
		return widget.NewLabel(err.Error())
	}
	decrypted, err := fio.DecryptContent(account, req.PayeeKey, req.Content, fio.ObtRequestType)
	if err != nil {
		return widget.NewLabel(err.Error())
	}
	reqData := make([]*widget.FormItem, 0)
	add := func(name string, value string) {
		if len(value) > 0 {
			a := widget.NewEntry()
			a.SetText(value)
			a.OnChanged = func(string) {
				a.SetText(value)
			}
			reqData = append(reqData, widget.NewFormItem(name, a))
		}
	}
	reqData = append(reqData, widget.NewFormItem("Request Envelope", layout.NewSpacer()))
	add("Request ID", strconv.FormatUint(id, 10))
	add("Time", req.Time.Format(time.UnixDate))
	add("Payer (To)", req.PayerFioAddress)
	add("", req.PayerKey)
	add("Payee (From)", req.PayeeFioAddress)
	add("", req.PayeeKey)
	reqData = append(reqData, widget.NewFormItem("", layout.NewSpacer()))
	reqData = append(reqData, widget.NewFormItem("Decrypted Request", layout.NewSpacer()))
	add("Payee Public Address", decrypted.Request.PayeePublicAddress)
	add("Amount", decrypted.Request.Amount)
	add("Chain Code", decrypted.Request.ChainCode)
	add("Token Code", decrypted.Request.TokenCode)
	add("Memo", decrypted.Request.Memo)
	add("Hash", decrypted.Request.Hash)
	add("Offline Url", decrypted.Request.OfflineUrl)

	errMsg := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Monospace: true})
	errIcon := fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(20, 20)), canvas.NewImageFromResource(theme.WarningIcon()))
	errIcon.Hide()
	respondBtn := &widget.Button{}
	respondBtn = widget.NewButtonWithIcon("Record Response", theme.MailReplyIcon(), func() {
		closing := make(chan interface{})
		d := dialog.NewCustom(
			fmt.Sprintf("Respond: Request ID %d (%s)", req.FioRequestId, req.PayeeFioAddress),
			"Cancel",
			RespondRequest(req, decrypted.Request, closing, account, api),
			Win,
		)
		go func() {
			<-closing
			close(closed)
			d.Hide()
		}()
		d.Show()
	})
	rejectBtn := &widget.Button{}
	rejectBtn = widget.NewButtonWithIcon("Reject", theme.DeleteIcon(), func() {
		rejectBtn.Disable()
		respondBtn.Disable()
		resp, err := api.SignPushActions(fio.NewRejectFndReq(account.Actor, strconv.FormatUint(id, 10)))
		if err != nil {
			errIcon.Show()
			errMsg.SetText(err.Error())
			errMsg.Refresh()
			rejectBtn.Enable()
			respondBtn.Enable()
			return
		}
		errIcon.Hide()
		errMsg.SetText("Done. Transaction ID: " + resp.TransactionID)
		refresh <- true
	})
	buttons := widget.NewVBox(
		widget.NewHBox(
			layout.NewSpacer(),
			respondBtn,
			rejectBtn,
			layout.NewSpacer(),
		),
		widget.NewHBox(layout.NewSpacer(), errIcon, errMsg, layout.NewSpacer()),
	)
	f := widget.NewForm(reqData...)
	return widget.NewVBox(
		widget.NewHBox(layout.NewSpacer(), f, layout.NewSpacer()),
		buttons,
	)
}

func RespondRequest(req *fio.FundsReqTableResp, decrypted *fio.ObtRequestContent, closed chan interface{}, account *fio.Account, api *fio.API) fyne.CanvasObject {
	var memo string
	if len(decrypted.Memo) > 0 {
		memo = "re: " + decrypted.Memo
	}
	record := &fio.ObtRecordContent{
		PayerPublicAddress: "",
		PayeePublicAddress: decrypted.PayeePublicAddress,
		Amount:             decrypted.Amount,
		ChainCode:          decrypted.ChainCode,
		TokenCode:          decrypted.TokenCode,
		Status:             "",
		ObtId:              strconv.FormatUint(req.FioRequestId, 10),
		Memo:               memo,
		Hash:               "",
		OfflineUrl:         "",
	}
	bytesRemaining := widget.NewLabel("")
	remaining := func(l *widget.Label) {
		content, err := record.Encrypt(account, req.PayeeKey)
		if err != nil {
			l.SetText(err.Error())
		}
		tooLarge := ""
		if 432-len(content) < 0 {
			tooLarge = "Content is too large! "
		}
		over := 432 - len(content)
		if over >= 0 {
			over = 0
		}
		l.SetText(fmt.Sprintf("%s%d bytes over (%d bytes after encryption and encoding) 432 max", tooLarge, over, len(content)))
	}
	respData := make([]*widget.FormItem, 0)
	add := func(name string, value string, disabled bool, updateField *string) {
		a := widget.NewEntry()
		a.SetText(value)
		if disabled {
			a.Disable()
		}
		a.OnChanged = func(string) {
			if updateField != nil {
				*updateField = a.Text
			}
			remaining(bytesRemaining)
		}
		respData = append(respData, widget.NewFormItem(name, a))
	}
	add("Request ID", strconv.FormatUint(req.FioRequestId, 10), true, nil)
	add("Payer (To)", req.PayerFioAddress, true, nil)
	add("Payee (From)", req.PayeeFioAddress, true, nil)
	respData = append(respData, widget.NewFormItem("", layout.NewSpacer()))
	add("Payer Public Key", "", false, &record.PayerPublicAddress)
	add("Payee Public Key", decrypted.PayeePublicAddress, true, nil)
	add("Chain Code", decrypted.ChainCode, true, nil)
	add("Token Code", decrypted.TokenCode, true, nil)
	add("Amount", decrypted.Amount, false, &record.Amount)
	add("Status", "", false, &record.Status)
	add("Memo", memo, false, &record.Memo)
	add("Hash", "", false, &record.Hash)
	add("Offline Url", "", false, &record.OfflineUrl)

	errMsg := widget.NewLabel("")
	sendResponse := &widget.Button{}
	sendResponse = widget.NewButtonWithIcon("Send Response", theme.ConfirmIcon(), func() {
		content, err := record.Encrypt(account, req.PayeeKey)
		if err != nil {
			errMsg.SetText("Encrypt response: " + err.Error())
			return
		}
		resp, err := api.SignPushActions(fio.NewRecordSend(account.Actor, strconv.FormatUint(req.FioRequestId, 10), req.PayerFioAddress, req.PayeeFioAddress, content))
		if err != nil {
			errMsg.SetText("Push Action: " + err.Error())
			return
		}
		errs.ErrChan <- "Success, txid: " + resp.TransactionID
		errMsg.SetText("Success, txid: " + resp.TransactionID)
		sendResponse.Disable()
		time.Sleep(2 * time.Second)
		close(closed)
	})

	remaining(bytesRemaining)
	return widget.NewVBox(
		bytesRemaining,
		widget.NewForm(respData...),
		errMsg,
		sendResponse,
	)
}

func NewRequest(account *fio.Account, api *fio.API) fyne.CanvasObject {
	n, _, err := account.GetNames(api)
	if err != nil {
		return widget.NewLabel(err.Error())
	}
	if n == 0 {
		return widget.NewLabel("You must have at least one FIO Name to send requests.")
	}
	send := &widget.Button{}
	var content, payerFio, payerPub, payeeFio string
	nfr := &fio.ObtRequestContent{}

	reqFormData := make([]*widget.FormItem, 0)
	add := func(name string, value string, disabled bool, updateField *string) {
		a := widget.NewEntry()
		a.SetText(value)
		if disabled {
			a.Disable()
		}
		a.OnChanged = func(string) {
			if updateField != nil {
				*updateField = a.Text
			}
		}
		reqFormData = append(reqFormData, widget.NewFormItem(name, a))
	}
	payeeNameSelect := widget.NewSelect(func() []string {
		names := make([]string, len(account.Addresses))
		for i := range account.Addresses {
			names[i] = account.Addresses[i].FioAddress
		}
		return names
	}(),
		func(s string) {
			payeeFio = s
		},
	)
	payeeNameSelect.SetSelected(payeeNameSelect.Options[0])
	reqFormData = append(reqFormData, widget.NewFormItem("Your FIO Name", payeeNameSelect))

	payerPubLabel := widget.NewLabelWithStyle(spaces, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	payerName := widget.NewEntry()
	invalid := func() {
		payerPubLabel.SetText(spaces)
		payerPub = ""
		send.Disable()
	}
	payerName.OnChanged = func(s string) {
		if !fio.Address(s).Valid() {
			invalid()
			return
		}
		pa, ok, _ := api.PubAddressLookup(fio.Address(s), "FIO", "FIO")
		if !ok || pa.PublicAddress == "" {
			invalid()
			return
		}
		payerPubLabel.SetText(pa.PublicAddress)
		payerPub = pa.PublicAddress
		payerFio = s
		send.Enable()
	}
	reqFormData = append(reqFormData, widget.NewFormItem("Recipient's FIO Name", payerName))
	reqFormData = append(reqFormData, widget.NewFormItem("", payerPubLabel))

	reqFormData = append(reqFormData, widget.NewFormItem("", layout.NewSpacer()))

	payeeRecvAddress := widget.NewEntry()
	payeeRecvAddress.OnChanged = func(s string) {
		nfr.PayeePublicAddress = s
	}
	reqFormData = append(reqFormData, widget.NewFormItem("Send to Address", payeeRecvAddress))

	chainSelect := &widget.SelectEntry{}
	tokenSelect := widget.NewSelectEntry(make([]string, 0))
	tokenSelect.OnChanged = func(s string) {
		nfr.TokenCode = s
		resp, found, _ := api.PubAddressLookup(fio.Address(payeeFio), chainSelect.Text, tokenSelect.Text)
		if !found {
			return
		}
		payeeRecvAddress.SetText(resp.PublicAddress)
	}
	chainSelect = widget.NewSelectEntry(GetChains())
	chainSelect.OnChanged = func(s string) {
		tokenSelect.SetOptions(GetTokens(s))
		tokenSelect.SetText(s)
		tokenSelect.Refresh()
		nfr.ChainCode = s
	}
	chainSelect.SetText("BTC")
	tokenSelect.SetText("BTC")
	reqFormData = append(reqFormData, widget.NewFormItem("Chain Code", chainSelect))
	reqFormData = append(reqFormData, widget.NewFormItem("Token Code", tokenSelect))

	warn := fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(20, 20)),
		canvas.NewImageFromResource(theme.WarningIcon()),
	)
	amountEntry := widget.NewEntry()
	amountEntry.SetText("0.0")
	amountEntry.OnChanged = func(s string) {
		fl, err := strconv.ParseFloat(s, 64)
		if err != nil || fl == 0 {
			warn.Show()
			return
		}
		warn.Hide()
		nfr.Amount = s
	}
	reqFormData = append(reqFormData, widget.NewFormItem("Amount", widget.NewHBox(amountEntry, warn)))
	add("Memo", "", false, &nfr.Memo)
	add("Hash", "", false, &nfr.Hash)
	add("Offline Url", "", false, &nfr.OfflineUrl)
	errLabel := widget.NewLabel("")
	sendRequest := func() {
		defer func() {
			go func() {
				// prevent accidental click click click
				time.Sleep(2 * time.Second)
				send.Enable()
			}()
		}()
		send.Disable()
		content, err = nfr.Encrypt(account, payerPub)
		if err != nil {
			errLabel.SetText(err.Error())
			errs.ErrChan <- err.Error()
			return
		}
		resp, err := api.SignPushActions(fio.NewFundsReq(account.Actor, payerFio, payeeFio, content))
		if err != nil {
			errLabel.SetText(err.Error())
			errs.ErrChan <- err.Error()
			return
		}
		errLabel.SetText("Success, txid: " + resp.TransactionID)
	}
	send = widget.NewButtonWithIcon("Send Request", theme.ConfirmIcon(), sendRequest)
	send.Disable()
	return widget.NewVBox(
		widget.NewForm(reqFormData...),
		widget.NewHBox(layout.NewSpacer(), errLabel, layout.NewSpacer()),
		send,
	)
}

var chainMux = sync.Mutex{}

func GetChains() []string {
	chainMux.Lock()
	defer chainMux.Unlock()
	result := make([]string, len(chainTokens))
	i := 0
	for k := range chainTokens {
		result[i] = k
		i += 1
	}
	sort.Strings(result)
	return result
}

func GetTokens(s string) []string {
	chainMux.Lock()
	defer chainMux.Unlock()
	if s == "" || chainTokens[s] == nil {
		return make([]string, 0)
	}
	return chainTokens[s]
}

var chainTokens = map[string][]string{
	"ABBC": {"ABBC"},
	"ADA":  {"ADA"},
	"ALGO": {"ALGO"},
	"ATOM": {"ATOM"},
	"BAND": {"BAND"},
	"BCH": {
		"BCH",
		"FLEX",
	},
	"BHD": {"BHD"},
	"BNB": {
		"ANKR",
		"BNB",
		"CHZ",
		"ERD",
		"ONE",
		"RUNE",
		"SWINGBY",
	},
	"BSV":  {"BSV"},
	"BTC":  {"BTC"},
	"BTM":  {"BTM"},
	"CET":  {"CET"},
	"CHX":  {"CHX"},
	"CKB":  {"CKB"},
	"DASH": {"DASH"},
	"DOGE": {"DOGE"},
	"DOT":  {"DOT"},
	"EOS":  {"EOS"},
	"ETC":  {"ETC"},
	"ETH": {
		"AERGO",
		"AKRO",
		"ALTBEAR",
		"ALTBULL",
		"BAND",
		"BAT",
		"BEPRO",
		"BNBBEAR",
		"BNBBULL",
		"BOLT",
		"BTCBEAR",
		"BTCBULL",
		"BTMX",
		"BVOL",
		"BXA",
		"CELR",
		"CET",
		"CHR",
		"COTI",
		"COVA",
		"CRO",
		"CVNT",
		"DAD",
		"DEEP",
		"DIA",
		"DOS",
		"DREP",
		"DUO",
		"ELF",
		"EOSBEAR",
		"EOSBULL",
		"ETH",
		"ETHBEAR",
		"ETHBULL",
		"EXCHBEAR",
		"EXCHBULL",
		"FET",
		"FRM",
		"FTM",
		"FTT",
		"GEEQ",
		"GT",
		"HT",
		"IBVOL",
		"INFT",
		"JRT",
		"KCS",
		"LAMB",
		"LAMBS",
		"LBA",
		"LFT",
		"LINK",
		"LTCBEAR",
		"LTCBULL",
		"LTO",
		"MATIC",
		"MITX",
		"MIX",
		"OKB",
		"OLT",
		"OM",
		"ORN",
		"PAX",
		"PROM",
		"QCX",
		"RNT",
		"SEELE",
		"SLV",
		"SRM",
		"STAKE",
		"STPT",
		"SWAP",
		"TOKO",
		"UAT",
		"USDC",
		"USDT",
		"VALOR",
		"VRA",
		"XRPBEAR",
		"XRPBULL",
		"ZRX",
	},
	"ETZ": {"ETZ"},
	"FIAT": {
		"ACH",
		"IBAN",
	},
	"FIO":  {"FIO"},
	"FSN":  {"FSN"},
	"HPB":  {"HPB"},
	"IOST": {"IOST"},
	"KAVA": {"KAVA"},
	"LTC":  {"LTC"},
	"LTO":  {"LTO"},
	"MHC":  {"MHC"},
	"NEO": {
		"GAS",
		"NEO",
	},
	"OLT":  {"OLT"},
	"OMNI": {"USDT"},
	"ONE":  {"ONE"},
	"ONT": {"ONG",
		"ONT"},
	"QTUM": {"QTUM"},
	"RVN":  {"RVN"},
	"SOL":  {"SOL"},
	"TRX": {
		"BTT",
		"TRX",
		"USDT",
	},
	"VET": {"VET"},
	"WAN": {
		"RVX",
		"WAN",
	},
	"XEM": {"XEM"},
	"XLM": {"XLM"},
	"XMR": {"XMR"},
	"XNS": {"XNS"},
	"XRP": {"XRP"},
	"XTZ": {"XTZ"},
	"YAP": {"YAP"},
	"ZEC": {"ZEC"},
	"ZIL": {"ZIL"},
}
