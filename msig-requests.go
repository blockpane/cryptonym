package cryptonym

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/canvas"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	fioassets "github.com/blockpane/cryptonym/assets"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

func MsigRequestsContent(api *fio.API, opts *fio.TxOptions, account *fio.Account) *widget.TabItem {
	pendingTab := widget.NewTabItem("Pending Requests",
		widget.NewScrollContainer(
			ProposalRows(0, 100, api, opts, account),
		),
	)
	return pendingTab
}

func requestBox(proposer string, requests []*fio.MsigApprovalsInfo, index int, proposalWindow fyne.Window, api *fio.API, opts *fio.TxOptions, account *fio.Account) fyne.CanvasObject {
	p := message.NewPrinter(language.AmericanEnglish)
	aFee := fio.GetMaxFee(fio.FeeMsigApprove)
	dFee := fio.GetMaxFee(fio.FeeMsigUnapprove)
	cFee := fio.GetMaxFee(fio.FeeMsigCancel)
	eFee := fio.GetMaxFee(fio.FeeMsigExec)
	proposalHash := eos.Checksum256{}

	refresh := func() {
		_, ai, err := api.GetApprovals(fio.Name(proposer), 10)
		if err != nil {
			errs.ErrChan <- errs.Detailed(err)
			return
		}
		requests = ai
	}

	approve := widget.NewButtonWithIcon(p.Sprintf("Approve %s %g", fio.FioSymbol, aFee), theme.ConfirmIcon(), func() {
		_, tx, err := api.SignTransaction(
			fio.NewTransaction([]*fio.Action{
				fio.NewMsigApprove(eos.AccountName(proposer), requests[index].ProposalName, account.Actor, proposalHash),
			}, opts),
			opts.ChainID, fio.CompressionNone,
		)
		if err != nil {
			errs.ErrChan <- errs.Detailed(err)
			resultPopup(err.Error(), proposalWindow)
			return
		}
		res, err := api.PushTransactionRaw(tx)
		if err != nil {
			errs.ErrChan <-errs.Detailed(err)
			resultPopup(err.Error(), proposalWindow)
			return
		}
		errs.ErrChan <- fmt.Sprintf("sending approval for proposal '%s' proposed by %s", requests[index].ProposalName, proposer)
		j, _ := json.MarshalIndent(res, "", "    ")
		refresh()
		proposalWindow.SetContent(
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*85)/100, (H*85)/100)),
				requestBox(proposer, requests, index, proposalWindow, api, opts, account),
			))
		proposalWindow.Content().Refresh()
		resultPopup(string(j), proposalWindow)
	})
	approve.Hide()
	deny := widget.NewButtonWithIcon(p.Sprintf("Un-Approve %s %g", fio.FioSymbol, dFee), theme.ContentUndoIcon(), func() {
		_, tx, err := api.SignTransaction(
			fio.NewTransaction([]*fio.Action{
				fio.NewMsigUnapprove(eos.AccountName(proposer), requests[index].ProposalName, account.Actor),
			}, opts),
			opts.ChainID, fio.CompressionNone,
		)
		if err != nil {
			errs.ErrChan <- errs.Detailed(err)
			resultPopup(err.Error(), proposalWindow)
			return
		}
		res, err := api.PushTransactionRaw(tx)
		if err != nil {
			errs.ErrChan <- errs.Detailed(err)
			resultPopup(err.Error(), proposalWindow)
			return
		}
		j, _ := json.MarshalIndent(res, "", "    ")
		errs.ErrChan <- fmt.Sprintf("withdrawing approval for proposal '%s' proposed by %s", requests[index].ProposalName, proposer)
		refresh()
		proposalWindow.SetContent(
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*85)/100, (H*85)/100)),
				requestBox(proposer, requests, index, proposalWindow, api, opts, account),
			))
		proposalWindow.Content().Refresh()
		resultPopup(string(j), proposalWindow)
	})
	deny.Hide()
	cancel := widget.NewButtonWithIcon(p.Sprintf("Cancel %s %g", fio.FioSymbol, cFee), theme.DeleteIcon(), func() {
		_, tx, err := api.SignTransaction(
			fio.NewTransaction([]*fio.Action{
				fio.NewMsigCancel(eos.AccountName(proposer), requests[index].ProposalName, account.Actor),
			}, opts),
			opts.ChainID, fio.CompressionNone,
		)
		if err != nil {
			errs.ErrChan <- err.Error()
			resultPopup(err.Error(), proposalWindow)
			return
		}
		res, err := api.PushTransactionRaw(tx)
		if err != nil {
			errs.ErrChan <- errs.Detailed(err)
			resultPopup(err.Error(), proposalWindow)
			return
		}
		j, _ := json.MarshalIndent(res, "", "    ")
		errs.ErrChan <- fmt.Sprintf("cancel proposal '%s'", requests[index].ProposalName)
		resultPopup(string(j), proposalWindow)
	})
	cancel.Hide()
	execute := widget.NewButtonWithIcon(p.Sprintf("Execute %s %g", fio.FioSymbol, eFee), fioassets.NewFioLogoResource(), func() {
		_, tx, err := api.SignTransaction(
			fio.NewTransaction([]*fio.Action{
				fio.NewMsigExec(eos.AccountName(proposer), requests[index].ProposalName, fio.Tokens(eFee), account.Actor),
			}, opts),
			opts.ChainID, fio.CompressionNone,
		)
		if err != nil {
			errs.ErrChan <- err.Error()
			resultPopup(err.Error(), proposalWindow)
			return
		}
		res, err := api.PushTransactionRaw(tx)
		if err != nil {
			errs.ErrChan <- err.Error()
			resultPopup(err.Error(), proposalWindow)
			return
		}
		j, _ := json.MarshalIndent(res, "", "    ")
		errs.ErrChan <- fmt.Sprintf("executing proposal '%s' proposed by %s", requests[index].ProposalName, proposer)
		refresh()
		proposalWindow.SetContent(
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*85)/100, (H*85)/100)),
				requestBox(proposer, requests, index, proposalWindow, api, opts, account),
			))
		proposalWindow.Content().Refresh()
		resultPopup(string(j), proposalWindow)
	})
	execute.Hide()
	if proposer == string(account.Actor) {
		cancel.Show()
	}
	if len(requests) <= index {
		return widget.NewHBox(
			layout.NewSpacer(),
			widget.NewLabelWithStyle("Requests table has changed, please refresh and try again.", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
		)
	}
	if requests[index].HasApproved(account.Actor) {
		deny.Show()
	}
	if requests[index].HasRequested(account.Actor) && !requests[index].HasApproved(account.Actor) {
		approve.Show()
	}
	approvers := make(map[string]bool)
	approvalWeightLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	proposalTitle := widget.NewHBox(
		layout.NewSpacer(),
		widget.NewLabel("Proposal Name: "),
		widget.NewLabelWithStyle(string(requests[index].ProposalName), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
			Win.Clipboard().SetContent(string(requests[index].ProposalName))
		}),
		layout.NewSpacer(),
	)
	proposalAuthor := widget.NewHBox(
		layout.NewSpacer(),
		widget.NewLabel("Proposal Author: "),
		widget.NewLabelWithStyle(proposer, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
			Win.Clipboard().SetContent(proposer)
		}),
		layout.NewSpacer(),
	)
	approversRows := widget.NewVBox(proposalTitle, proposalAuthor, approvalWeightLabel)
	producers, pErr := api.GetProducerSchedule()
	var top21Count int
	isProd := func(name eos.AccountName) string {
		result := make([]string, 0)
		if pErr != nil || producers == nil {
			return ""
		}
		for _, p := range producers.Active.Producers {
			if p.AccountName == name {
				result = append(result, "Top 21 Producer")
				break
			}
		}
		if account.Actor == name {
			result = append(result, "Current Actor")
		}
		if string(name) == proposer {
			result = append(result, "Proposal Author")
		}
		return strings.Join(result, " ~ ")
	}

	for _, approver := range requests[index].RequestedApprovals {
		approvers[string(approver.Level.Actor)] = false
	}
	for _, approver := range requests[index].ProvidedApprovals {
		approvers[string(approver.Level.Actor)] = true
	}
	approversRows.Append(widget.NewHBox(
		layout.NewSpacer(), approve, deny, cancel, execute, layout.NewSpacer(),
	))
	approversRows.Append(
		fyne.NewContainerWithLayout(layout.NewGridLayout(3),
			fyne.NewContainerWithLayout(layout.NewGridLayout(2),
				widget.NewLabelWithStyle("Approved", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
				widget.NewLabelWithStyle("Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			),
			widget.NewLabelWithStyle("Account", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
		),
	)

	approversSorted := func() []string {
		s := make([]string, 0)
		for k := range approvers {
			s = append(s, k)
		}
		sort.Strings(s)
		return s
	}()
	var checked int
	for _, k := range approversSorted {
		// actor, fio address, has approved, is produce
		//hasApproved := theme.CancelIcon()
		hasApproved := theme.CheckButtonIcon()
		asterisk := ""
		if approvers[k] {
			//hasApproved = theme.ConfirmIcon()
			hasApproved = theme.CheckButtonCheckedIcon()
			checked += 1
			for _, p := range producers.Active.Producers {
				if p.AccountName == eos.AccountName(k) {
					top21Count += 1
					asterisk = "*"
					break
				}
			}
		}
		top21Label := widget.NewLabel(asterisk)
		var firstName string
		n, ok, _ := api.GetFioNamesForActor(k)
		if ok && len(n.FioAddresses) > 0 {
			firstName = n.FioAddresses[0].FioAddress
		}
		deref := &k
		actor := *deref
		copyButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
			Win.Clipboard().SetContent(actor)
		})
		approversRows.Append(fyne.NewContainerWithLayout(layout.NewGridLayout(3),
			fyne.NewContainerWithLayout(layout.NewGridLayout(2),
				widget.NewHBox(
					layout.NewSpacer(),
					top21Label,
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(32, 32)),
						canvas.NewImageFromResource(hasApproved),
					)),
				widget.NewLabel(firstName),
			),
			widget.NewHBox(
				layout.NewSpacer(),
				copyButton,
				widget.NewLabelWithStyle(k, fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true}),
			),
			widget.NewLabelWithStyle(isProd(eos.AccountName(k)), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		))

	}

	type actionActor struct {
		Actor string `json:"actor"`
	}
	// will use for counting vote weights ...
	actorMap := make(map[string]bool)
	actions := make([]msigActionInfo, 0)
	actionString := ""
	tx, err := api.GetProposalTransaction(eos.AccountName(proposer), requests[index].ProposalName)
	if err != nil {
		return widget.NewHBox(widget.NewLabelWithStyle(err.Error(), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	} else {
		proposalHash = tx.ProposalHash
		for _, action := range tx.PackedTransaction.Actions {
			abi, err := api.GetABI(action.Account)
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			decoded, err := abi.ABI.DecodeAction(action.HexData, action.Name)
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			actions = append(actions, msigActionInfo{
				Action:       decoded,
				Account:      string(action.Account),
				Name:         string(action.Name),
				ProposalHash: tx.ProposalHash,
			})
			aActor := &actionActor{}
			err = json.Unmarshal(decoded, aActor)
			if err == nil && aActor.Actor != "" {
				actorMap[aActor.Actor] = true
			}
		}
		a, err := json.MarshalIndent(actions, "", "  ")
		if err != nil {
			errs.ErrChan <- err.Error()
		} else {
			actionString = string(a)
		}
	}
	hasApprovals := true
	approvalsNeeded := make([]string, 0)
	have := "have"
	var privAction bool
	for a := range PrivilegedActions {
		sa := strings.Split(a, "::")
		if len(sa) != 2 {
			continue
		}
		if string(tx.PackedTransaction.Actions[0].Name) == sa[1] {
			privAction = true
			break
		}
	}
	if privAction {
		if checked == 1 {
			have = "has"
		}
		var required int
		type tp struct {
			Producer string `json:"producer"`
		}
		rows := make([]tp, 0)
		gtr, err := api.GetTableRows(eos.GetTableRowsRequest{
			Code:  "eosio",
			Scope: "eosio",
			Table: "topprods",
			Limit: 21,
			JSON:  true,
		})
		if err != nil {
			errs.ErrChan <- err.Error()
			required = 15
		} else {
			_ = json.Unmarshal(gtr.Rows, &rows)
			if len(rows) == 0 || len(rows) == 21 {
				required = 15
			} else {
				required = (len(rows) / 2) + ((len(rows) / 2) / 2)
			}
		}
		var top21Voted string
		if top21Count > 0 {
			top21Voted = fmt.Sprintf(" - (%d are Top 21 Producers)", top21Count)
		}
		approvalsNeeded = append(approvalsNeeded, fmt.Sprintf("Account %s requires %d approvals, %d %s been provided%s", tx.PackedTransaction.Actions[0].Account, required, checked, have, top21Voted))
		if checked < required {
			hasApprovals = false
		}
	} else {
		for msigAccount := range actorMap {
			needs, has, err := getVoteWeight(msigAccount, requests[index].ProvidedApprovals, api)
			if err != nil {
				errs.ErrChan <- err.Error()
				hasApprovals = false
			}
			if has == 1 {
				have = "has"
			}
			approvalsNeeded = append(approvalsNeeded, fmt.Sprintf("Account %s requires %d approvals, %d %s been provided", msigAccount, needs, has, have))
			if has < needs {
				hasApprovals = false
			}
		}
	}
	if hasApprovals {
		execute.SetText(p.Sprintf("Execute %s %g", fio.FioSymbol, eFee))
		execute.Show()
	}
	approvalWeightLabel.SetText(strings.Join(approvalsNeeded, "\n"))
	approvalWeightLabel.Refresh()
	actionEntry := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	actionLines := strings.Split(actionString, "\n")
	for i := range actionLines {
		if len(actionLines[i]) > 128 {
			actionLines[i] = actionLines[i][:125] + "..."
		}
	}
	actionEntry.SetText(strings.Join(actionLines, "\n"))
	actionBox := widget.NewGroup("Transaction", widget.NewHBox(
		layout.NewSpacer(),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*75)/100, actionEntry.MinSize().Height)),
			actionEntry,
		),
		layout.NewSpacer(),
	))
	return widget.NewGroupWithScroller(string(requests[index].ProposalName), approversRows, actionBox)
}

type msigActionInfo struct {
	Account      string          `json:"account"`
	Name         string          `json:"name"`
	Action       json.RawMessage `json:"action"`
	ProposalHash eos.Checksum256 `json:"proposal_hash"`
}

func getVoteWeight(account string, providedApprovals []fio.MsigApproval, api *fio.API) (required int, current int, err error) {
	acc, err := api.GetFioAccount(account)
	if err != nil {
		return 0, 0, nil
	}
	weights := make(map[string]int)
	for _, a := range acc.Permissions {
		if a.PermName == "active" && a.RequiredAuth.Accounts != nil && len(a.RequiredAuth.Accounts) > 0 {
			required = int(a.RequiredAuth.Threshold)
			for _, owner := range a.RequiredAuth.Accounts {
				weights[string(owner.Permission.Actor)] = int(owner.Weight)
			}
		}
	}
	for _, approved := range providedApprovals {
		current = current + weights[string(approved.Level.Actor)]
	}
	return
}

func resultPopup(result string, window fyne.Window) {
	mle := widget.NewMultiLineEntry()
	mle.SetText(result)
	dialog.ShowCustom("Transaction Result", "Done",
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*50)/100, (H*50)/100)),
			widget.NewScrollContainer(
				mle,
			),
		),
		window,
	)
}

type msigProposalRow struct {
	ProposalName      eos.Name `json:"proposal_name"`
	PackedTransaction string    `json:"packed_transaction"`
}

func ProposalRows(offset int, limit int, api *fio.API, opts *fio.TxOptions, account *fio.Account) *widget.Box {
	_, proposals, err := api.GetProposals(offset, limit)
	if err != nil {
		errs.ErrChan <- err.Error()
		return widget.NewHBox(widget.NewLabel(err.Error()))
	}
	refButton := &widget.Button{}
	refButton = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		refButton.Disable()
		MsigLastTab = 0
		go func() {
			MsigRefreshRequests <- true
		}()
	})
	//onlyPriv := widget.NewCheck("Only Privileged", func(b bool) {})
	//onlyPriv.SetChecked(true)
	//last14 := widget.NewCheck("Only show last 14 days", func(b bool) {})
	//last14.SetChecked(true)

	vb := widget.NewVBox()
	vb.Append(widget.NewHBox(
		//layout.NewSpacer(), onlyPriv, last14, refButton, layout.NewSpacer(),
		layout.NewSpacer(), refButton, layout.NewSpacer(),
	),
	)
	proposalsSorted := func() []string {
		p := make([]string, 0)
		for k := range proposals {
			p = append(p, k)
		}
		sort.Strings(p)
		return p
	}()
	for _, proposer := range proposalsSorted {
		more, approvalsInfo, err := api.GetApprovals(fio.Name(proposer), 10)
		if err != nil {
			errs.ErrChan <- err.Error()
			continue
		}
		names, found, err := api.GetFioNamesForActor(proposer)
		if err != nil {
			errs.ErrChan <- err.Error()
			continue
		}
		fioAddresses := make([]string, 0)
		if found {
			for _, n := range names.FioAddresses {
				fioAddresses = append(fioAddresses, n.FioAddress)
			}
		}
		var fioAddrs string
		if len(fioAddresses) > 0 {
			fioAddrs = strings.Join(fioAddresses, ", ")
			if len(fioAddrs) > 32 {
				fioAddrs = fioAddrs[:28] + "..."
			}
		}
		var sep string
		if fioAddrs != "" {
			sep = " – "
		}
		groupRows := fyne.NewContainerWithLayout(layout.NewGridLayout(2))
		if more {
			groupRows.AddObject(
				widget.NewLabel("Account has more than 10 proposals, not all are shown."),
			)
			groupRows.AddObject(layout.NewSpacer())
		}
		group := widget.NewGroup(fmt.Sprintf(" (%s)%s%s ", proposer, sep, fioAddrs), groupRows)
		sort.Slice(approvalsInfo, func(i, j int) bool {
			a, _ := eos.StringToName(string(approvalsInfo[i].ProposalName))
			b, _ := eos.StringToName(string(approvalsInfo[j].ProposalName))
			return a < b
		})
		for index, prop := range approvalsInfo {
			hasNeeds := fmt.Sprintf("%d of %d approvals", len(prop.ProvidedApprovals), len(prop.ProvidedApprovals)+len(prop.RequestedApprovals))
			//gpt, err := api.GetProposalTransaction(eos.AccountName(proposer), prop.ProposalName)
			j, err := json.Marshal(&fio.GetTableRowsOrderRequest{
				Code:       "eosio.msig",
				Scope:      proposer,
				Table:      "proposal",
				LowerBound: string(prop.ProposalName),
				UpperBound: string(prop.ProposalName),
				Limit:      1,
				KeyType:    "name",
				Index:      "1",
				JSON:       true,
				Reverse:    false,
			})
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			client := &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyFromEnvironment,
					DialContext: (&net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 30 * time.Second,
						DualStack: true,
					}).DialContext,
					MaxIdleConns:          100,
					IdleConnTimeout:       90 * time.Second,
					TLSHandshakeTimeout:   10 * time.Second,
					ExpectContinueTimeout: 1 * time.Second,
					DisableKeepAlives:     true, // default behavior, because of `nodeos`'s lack of support for Keep alives.
					ReadBufferSize: 1024 * 1024,
				},
			}
			resp, err := client.Post(api.BaseURL+"/v1/chain/get_table_rows", "application/json", bytes.NewReader(j))
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			//body, err := ioutil.ReadAll(resp.Body)
			//buf := bytes.NewBuffer(nil)
			copyBuf := make([]byte, 16384)
			//resp.Body.Read()
			//_, err = io.CopyBuffer(buf, resp.Body, copyBuf)
			//if err != nil {
			//	errs.ErrChan <- err.Error()
			//	continue
			//}
			body := make([]byte, 0)
			var e error
			var n int
			for {
				n, e = resp.Body.Read(copyBuf)
				if e != nil {
					if n > 0 {
						body = append(body, copyBuf[:n]...)
					}
					errs.ErrChan <- e.Error()
					break
				}
				body = append(body, copyBuf[:n]...)
			}
			resp.Body.Close()
			tableRows := &eos.GetTableRowsResp{}
			err = json.Unmarshal(body, tableRows)
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			gpts := make([]*msigProposalRow, 0)
			err = json.Unmarshal(tableRows.Rows, &gpts)
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			if len(gpts) == 0 {
				errs.ErrChan <- "got empty result from query"
				continue
			}
			txBytes, err := hex.DecodeString(gpts[0].PackedTransaction)
			decoder := eos.NewDecoder(txBytes)
			tx := &eos.Transaction{}
			err = decoder.Decode(tx)
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			h := sha256.New()
			_, err = h.Write(txBytes)
			if err != nil {
				errs.ErrChan <- err.Error()
				continue
			}
			sum := h.Sum(nil)
			gpt := &fio.MsigProposal{
				ProposalName: prop.ProposalName,
				PackedTransaction: tx,
				ProposalHash: sum,
			}
			for _, action := range gpt.PackedTransaction.Actions {
				a, err := api.GetABI(action.Account)
				if err != nil {
					errs.ErrChan <- err.Error()
					continue
				}
				derefIdx := &index
				idx := *derefIdx
				derefProp := &proposer
				newProp := *derefProp
				view := func() *widget.Button {
					return widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
						proposalWindow := App.NewWindow(newProp + " - " + string(prop.ProposalName))
						proposalWindow.SetContent(
							fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize((W*85)/100, (H*85)/100)),
								requestBox(newProp, approvalsInfo, idx, proposalWindow, api, opts, account),
							))
						proposalWindow.SetFixedSize(true)
						proposalWindow.SetOnClosed(func() {
							Win.RequestFocus()
						})
						proposalWindow.Show()
					})
				}()
				decoded, err := a.ABI.DecodeAction(action.HexData, action.Name)
				if err != nil {
					errs.ErrChan <- err.Error()
				}
				details := make(map[string]interface{})
				// don't try to decode an empty action
				if len(decoded) > 0 {
					err = json.Unmarshal(decoded, &details)
					if err != nil {
						errs.ErrChan <- err.Error()
						continue
					}
				}
				ds := make([]string, 0)
				keys := make([]string, 0)
				for k := range details {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if k == "tpid" || k == "max_fee" || k == "actor" {
						continue
					}
					ds = append(ds, fmt.Sprintf("%s: %+v", k, details[k]))
				}
				summaryCols := widget.NewHBox(
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(150, 30)), view),
					fyne.NewContainerWithLayout(layout.NewGridLayout(3),
						widget.NewHBox(layout.NewSpacer(), widget.NewLabelWithStyle(string(prop.ProposalName), fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}), layout.NewSpacer()),
						widget.NewHBox(layout.NewSpacer(), widget.NewLabelWithStyle(fmt.Sprintf("%16s", action.Name), fyne.TextAlignCenter, fyne.TextStyle{Monospace: true}), layout.NewSpacer()),
						widget.NewHBox(layout.NewSpacer(), widget.NewLabelWithStyle(hasNeeds, fyne.TextAlignCenter, fyne.TextStyle{}), layout.NewSpacer()),
					))
				txSummary := strings.Join(ds, ", ")
				if len(txSummary) > 64 {
					txSummary = txSummary[:60] + "..."
				}
				groupRows.AddObject(summaryCols)
				groupRows.AddObject(widget.NewLabelWithStyle(txSummary, fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true}))
			}
		}
		vb.Append(group)
		vb.Append(widget.NewHBox(widget.NewLabel(" ")))
	}
	return vb
}
