package cryptonym

import (
	"errors"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/widget"
	fioassets "github.com/blockpane/cryptonym/assets"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/blockpane/cryptonym/fuzzer"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	FormState    = NewAbi(0)
	bombsAway    = &widget.Button{}
	txWindowOpts = &txResultOpts{
		gone:   true,
	}
)

func ResetTxResult() {
	if txWindowOpts.window != nil {
		txWindowOpts.window.Close()
	}
	txWindowOpts.window = nil
	txWindowOpts.window = App.NewWindow("Tx Results")
	txWindowOpts.gone = true
	txWindowOpts.window.SetContent(layout.NewSpacer())
	txWindowOpts.window.Show()
	go func() {
		time.Sleep(100 * time.Millisecond)
		txWindowOpts.window.Hide()
	}()
}

// GetAbiForm returns the fyne form for editing the request, it also handles state tracking via
// the FormState which is later used to build the transaction.
func GetAbiForm(action string, account *fio.Account, api *fio.API, opts *fio.TxOptions) (fyne.CanvasObject, error) {
	if api.HttpClient == nil {
		return widget.NewVBox(), nil
	}
	accountAction := strings.Split(action, "::")
	if len(accountAction) != 2 {
		e := "couldn't parse account and action for " + action
		errs.ErrChan <- e
		return nil, errors.New(e)
	}
	abi, err := api.GetABI(eos.AccountName(accountAction[0]))
	if err != nil {
		errs.ErrChan <- err.Error()
		return nil, err
	}
	abiStruct := abi.ABI.StructForName(accountAction[1])
	form := widget.NewForm()

	abiState := NewAbi(len(abiStruct.Fields))
	abiState.Contract = accountAction[0]
	abiState.Action = accountAction[1]
	for i, deRef := range abiStruct.Fields {
		fieldRef := &deRef
		field := *fieldRef

		// input field
		inLabel := widget.NewLabel("Input:")
		if os.Getenv("ADVANCED") == "" {
			inLabel.Hide()
		}
		in := widget.NewEntry()
		in.SetText(defaultValues(accountAction[0], accountAction[1], field.Name, field.Type, account, api))
		inputBox := widget.NewHBox(
			inLabel,
			in,
		)
		in.OnChanged = func(s string) {
			FormState.UpdateInput(field.Name, in)
		}

		// abi type
		typeSelect := &widget.Select{}
		typeSelect = widget.NewSelect(abiSelectTypes(field.Type), func(s string) {
			FormState.UpdateType(field.Name, typeSelect)
		})
		typeSelect.SetSelected(field.Type)
		if os.Getenv("ADVANCED") == "" {
			typeSelect.Hide()
		}

		// count field, hidden by default
		num := &widget.Select{}
		num = widget.NewSelect(bytesLen, func(s string) {
			FormState.UpdateLen(field.Name, num)
		})
		num.Hide()

		// variant field
		variation := &widget.Select{}
		variation = widget.NewSelect(formVar, func(s string) {
			showNum, numVals, sel := getLength(s)
			if showNum {
				num.Show()
			} else {
				num.Hide()
			}
			num.Options = numVals
			num.SetSelected(sel)
			FormState.UpdateLen(field.Name, num)
			FormState.UpdateVariation(field.Name, variation)
		})
		if os.Getenv("ADVANCED") == "" {
			variation.Hide()
		}

		// options for fuzzer
		sendAs := &widget.Select{}
		sendAs = widget.NewSelect(sendAsSelectTypes, func(send string) {
			if !strings.Contains(send, "form value") {
				inputBox.Hide()
			} else {
				inputBox.Show()
			}
			var sel string
			variation.Options, sel = sendAsVariant(send)
			variation.SetSelected(sel)
			FormState.UpdateSendAs(field.Name, sendAs)
		})
		sendAs.SetSelected("form value")
		if os.Getenv("ADVANCED") == "" {
			sendAs.Hide()
		}

		form.Append(field.Name,
			widget.NewVBox(
				fyne.NewContainerWithLayout(layout.NewGridLayout(5),
					typeSelect,
					sendAs,
					variation,
					num,
				),
				inputBox,
			),
		)
		//name := field.Name
		abiState.Update(&i, AbiFormItem{
			Contract:  accountAction[0],
			Action:    accountAction[1],
			Name:      &field.Name,
			Type:      typeSelect,
			SendAs:    sendAs,
			Variation: variation,
			Input:     in,
			Len:       num,
			Order:     i,
		})
		if strings.HasPrefix(in.Text, "{") || strings.HasPrefix(in.Text, "[{") {
			variation.SetSelected("json -> struct")
			//in.Lock()
			in.MultiLine = true
			//in.Unlock()
		}
		if field.Name == "amount" || field.Name == "max_fee" {
			variation.SetSelected("FIO -> suf")
			if !strings.Contains(in.Text, ".") {
				in.SetText("10,000.00")
			}
		}
	}

	hideFailed := widget.NewCheck("Hide Failed", func(b bool) {})
	hideSuccess := widget.NewCheck("Hide Successful", func(b bool) {})
	zlibPack := widget.NewCheck("Pack With ZLib", func(b bool) {
		useZlib = b
	})
	zlibPack.Checked = useZlib
	zlibPack.Refresh()
	threadLabel := widget.NewLabel("Worker Count: ")
	threadLabel.Hide()
	threads := widget.NewSelect([]string{"1", "2", "4", "6", "8", "12", "16"}, func(s string) {})
	threads.SetSelected("1")
	threads.Hide()
	count := widget.NewEntry()
	count.SetText("1")
	delaySec := widget.NewEntry()
	delaySec.SetText(fmt.Sprintf("%d", delayTxSec))
	if !deferTx {
		delaySec.Hide()
	}
	delaySec.OnChanged = func(s string) {
		i, err := strconv.Atoi(s)
		if err != nil {
			errs.ErrChan <- "error converting delay time to int, setting to 0!"
			delayTxSec = 0
			delaySec.SetText("0")
			delaySec.Refresh()
		}
		delayTxSec = i
	}
	deferCheck := &widget.Check{}
	deferCheck = widget.NewCheck("Delay Transaction", func(b bool) {
		if b {
			deferCheck.Text = "Delay Transaction (seconds:)"
			deferCheck.Refresh()
			deferTx = true
			delaySec.Show()
			return
		}
		deferCheck.Text = "Delay Transaction"
		deferCheck.Refresh()
		deferTx = false
		delaySec.Hide()
	})
	if deferTx {
		deferCheck.Text = "Delay Transaction (seconds:)"
		deferCheck.Refresh()
	}
	deferCheck.Checked = deferTx
	deferCheck.Refresh()
	infinite := widget.NewCheck("Loop", func(b bool) {
		if b {
			count.Disable()
			threadLabel.Show()
			threads.Show()
			return
		}
		count.Enable()
		threadLabel.Hide()
		threads.Hide()
		threads.SetSelected("1")
	})

	err = EndPoints.Update(Uri, true)
	if err != nil {
		errs.ErrChan <- "Could not update api endpoint list: " + err.Error()
	}
	actionEndPointActive = "/v1/chain/push_transaction"
	apiEndpoint := widget.NewSelect(EndPoints.Apis, func(s string) {
		actionEndPointActive = s
	})
	apiEndpoint.SetSelected("/v1/chain/push_transaction")
	apiEndpoint.Refresh()

	// multisig options:
	randProposal := func() string {
		var s string
		for i := 0; i < 12; i++ {
			b := []byte{uint8(rand.Intn(26) + 97)}
			s = s + string(b)
		}
		return s
	}
	requested := widget.NewEntry()
	requested.SetText(getSigners(Settings.MsigAccount, api))
	msig := &widget.Box{}
	wrap := &widget.Box{}
	proposer := widget.NewEntry()
	proposer.SetText(Settings.MsigAccount)
	innerActionActor := widget.NewEntry()
	innerActionActor.SetText("eosio")
	innerActionActor.Hide()
	innerActionLabel := widget.NewLabel("Actor for inner action: ")
	innerActionLabel.Hide()
	wrapCheck := widget.NewCheck("Wrap Msig", func(b bool) {
		if b {
			innerActionActor.Show()
			innerActionLabel.Show()
			proposer.SetText("eosio.wrap")
			return
		}
		proposer.SetText(Settings.MsigAccount)
		innerActionActor.Hide()
		innerActionLabel.Hide()
	})
	proposer.OnChanged = func(s string) {
		requested.SetText(getSigners(s, api))
		if wrapCheck.Checked {
			s = innerActionActor.Text
		}
		for _, row := range FormState.Rows {
			if *row.Name == "actor" {
				row.Input.SetText(s)
				row.Input.Refresh()
				break
			}
		}
		go func() {
			time.Sleep(100 * time.Millisecond)
			requested.Refresh()
			msig.Refresh()
		}()
	}
	proposalName := widget.NewEntry()
	proposalName.SetText(randProposal())
	proposalName.Hide()
	proposalRand := widget.NewCheck("Random Name", func(b bool) {
		if b {
			proposalName.Hide()
		} else {
			proposalName.Show()
		}
	})
	proposalRand.SetChecked(true)

	msig = widget.NewHBox(
		widget.NewLabelWithStyle("Proposal Name: ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		proposalRand,
		proposalName,
		widget.NewLabelWithStyle("Multi-Sig Account: ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		proposer,
		widget.NewLabelWithStyle("Requested Signers: ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(380, 44)),
			widget.NewScrollContainer(requested),
		),
		layout.NewSpacer(),
	)
	msig.Hide()
	forcedSpace := widget.NewHBox(
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(20, 100)),
			layout.NewSpacer(),
		),
	)

	wrap = widget.NewHBox(
		wrapCheck,
		innerActionLabel,
		innerActionActor,
	)
	wrap.Hide()
	proposeCheck := widget.NewCheck("Propose", func(b bool) {
		if b {
			msig.Show()
			//wrap.Show()
		} else {
			msig.Hide()
			wrap.Hide()
		}
	})

	bombsAway = widget.NewButtonWithIcon("Send", fioassets.NewFioLogoResource(), func() {
		fuzzer.ResetIncrement()
		errs.ErrChan <- "generating transaction"
		repeat, err := strconv.Atoi(count.Text)
		if err != nil {
			repeat = 1
		}
		txWindowOpts.msig = proposeCheck.Checked
		txWindowOpts.msigSigners = requested.Text
		txWindowOpts.msigAccount = proposer.Text
		txWindowOpts.repeat = repeat
		txWindowOpts.loop = infinite.Checked
		txWindowOpts.threads = threads.Selected
		txWindowOpts.hideFail = hideFailed.Checked
		txWindowOpts.hideSucc = hideSuccess.Checked
		txWindowOpts.wrap = wrapCheck.Checked
		txWindowOpts.wrapActor = innerActionActor.Text
		if proposalRand.Checked {
			txWindowOpts.msigName = randProposal
		} else {
			txWindowOpts.msigName = func() string {
				return proposalName.Text
			}
		}
		TxResultsWindow(txWindowOpts, api, opts, account)
	})

	reqToSend := widget.NewLabel("Requests to send")
	if os.Getenv("ADVANCED") == "" {
		reqToSend.Hide()
		count.Hide()
		infinite.Hide()
		threadLabel.Hide()
		threads.Hide()
		hideFailed.Hide()
		hideSuccess.Hide()
		zlibPack.Hide()
		deferCheck.Hide()
		delaySec.Hide()
	}
	bottom := widget.NewHBox(
		widget.NewLabel(" "),
		bombsAway,
		reqToSend,
		count,
		infinite,
		threadLabel,
		threads,
		hideFailed,
		hideSuccess,
		zlibPack,
		deferCheck,
		delaySec,
		proposeCheck,
	)
	newRowName := widget.NewEntry()
	newRowName.SetPlaceHolder("New Row Name")
	label := widget.NewLabel(action)
	label.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	apiEndLabel := widget.NewLabel("API Endpoint")
	addRowButton := abiState.AddNewRowButton(newRowName, account, form)
	//topSpace := layout.NewSpacer()
	if os.Getenv("ADVANCED") == "" {
		apiEndLabel.Hide()
		apiEndpoint.Hide()
		addRowButton.Hide()
		newRowName.Hide()
		//topSpace.Hide()
	}
	content := widget.NewVScrollContainer(
		widget.NewVBox(
			label,
			layout.NewSpacer(),
			form,
			layout.NewSpacer(),
			bottom,
			msig,
			wrap,
			widget.NewHBox(
				apiEndLabel,
				apiEndpoint,
				layout.NewSpacer(),
				addRowButton,
				newRowName,
				layout.NewSpacer(),
			),
			forcedSpace,
		),
	)

	abiState.mux.Lock()
	FormState = abiState
	abiState.mux.Unlock()
	return content, nil
}
