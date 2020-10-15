package cryptonym

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"math"
	"sort"
	"strconv"
	"sync"
)

type FioActions struct {
	sync.RWMutex
	Index   []string
	Actions map[string][]string
}

type contracts struct {
	Owner string `json:"owner"`
}

func GetAccountSummary(api *fio.API) (*FioActions, error) {

	table, err := api.GetTableRows(eos.GetTableRowsRequest{
		Code:  "eosio",
		Scope: "eosio",
		Table: "abihash",
		Limit: uint32(math.MaxUint32),
		JSON:  true,
	})
	if err != nil {
		return nil, err
	}
	result := make([]contracts, 0)
	err = json.Unmarshal(table.Rows, &result)
	if err != nil {
		return nil, err
	}
	// FIXME: reading the abihash table isn't returning everything because of how the chain is boostrapped.
	// for now, appending a list of known contracts if not found :(
	defaults := []contracts{
		{Owner: "eosio"},
		//{Owner: "eosio.bios"},
		{Owner: "eosio.msig"},
		{Owner: "eosio.wrap"},
		{Owner: "fio.address"},
		//{Owner: "fio.common"},
		{Owner: "fio.fee"},
		{Owner: "fio.foundatn"},
		{Owner: "fio.reqobt"},
		//{Owner: "fio.system"},
		{Owner: "fio.token"},
		{Owner: "fio.tpid"},
		{Owner: "fio.treasury"},
		//{Owner: "fio.whitelst"},
	}
	for _, def := range defaults {
		func() {
			for _, found := range result {
				if def.Owner == found.Owner {
					return
				}
			}
			result = append(result, def)
		}()
	}

	actions := FioActions{
		Actions: make(map[string][]string),
	}

	// sort by account name
	func() {
		sorted := make([]string, 0)
		resultTemp := make([]contracts, 0)
		sortMap := make(map[string]int)
		for i, c := range result {
			sorted = append(sorted, c.Owner)
			sortMap[c.Owner] = i
		}
		sort.Strings(sorted)
		for _, ascending := range sorted {
			resultTemp = append(resultTemp, result[sortMap[ascending]])
		}
		result = resultTemp
	}()

	for _, name := range result {
		bi, err := api.GetABI(eos.AccountName(name.Owner))
		if err != nil {
			errs.ErrChan <- "problem while loading abi: " + err.Error()
			continue
		}
		actionList := make(map[string]bool, 0)
		for _, name := range bi.ABI.Actions {
			actionList[string(name.Name)] = true
		}
		if actions.Actions[name.Owner] == nil {
			actions.Actions[name.Owner] = make([]string, 0)
			actions.Index = append(actions.Index, name.Owner)
		}
		for a := range actionList {
			actions.Actions[name.Owner] = append(actions.Actions[name.Owner], a)
		}
		func() {
			tableList := make([]string, 0)
			for _, table := range bi.ABI.Tables {
				tableList = append(tableList, string(table.Name))
			}
			if len(tableList) == 0 {
				return
			}
			TableIndex.Add(name.Owner, tableList)
		}()
	}
	return &actions, nil
}

type TableBrowserIndex struct {
	mux     sync.RWMutex
	tables  map[string][]string
	created bool
}

func NewTableIndex() *TableBrowserIndex {
	return &TableBrowserIndex{tables: make(map[string][]string)}
}

func (tb *TableBrowserIndex) IsCreated() bool {
	return tb.created
}

func (tb *TableBrowserIndex) SetCreated(b bool) {
	tb.created = b
}

func (tb *TableBrowserIndex) Add(contract string, tables []string) (ok bool) {
	if contract == "" || len(tables) == 0 {
		return false
	}
	tb.mux.Lock()
	defer tb.mux.Unlock()
	sort.Strings(tables)
	tb.tables[contract] = tables
	return true
}

func (tb *TableBrowserIndex) Get(contract string) (tables []string) {
	tb.mux.RLock()
	defer tb.mux.RUnlock()
	if tb.tables[contract] == nil || len(tb.tables[contract]) == 0 {
		errs.ErrChan <- contract + " doesn't have any tables?"
		return []string{""}
	}
	return tb.tables[contract]
}

func (tb *TableBrowserIndex) List() []string {
	l := make([]string, 0)
	tb.mux.RLock()
	defer tb.mux.RUnlock()
	for tableName := range tb.tables {
		l = append(l, tableName)
	}
	sort.Strings(l)
	return l
}

func GetTableBrowser(w int, h int, api *fio.API) (tab *widget.Box, ok bool) {
	var getRows func()
	page := widget.NewEntry()
	page.SetText("1")
	page.Disable()
	rowsPerPage := widget.NewEntry()
	rowsPerPage.SetText("10")

	getRowsPerPage := func() uint32 {
		i, e := strconv.Atoi(rowsPerPage.Text)
		if e != nil {
			rowsPerPage.SetText("10")
			rowsPerPage.Refresh()
			return 10
		}
		return uint32(i)
	}

	result := widget.NewMultiLineEntry()
	submit := widget.NewButtonWithIcon("Query", theme.SearchIcon(), func() {
		getRows()
	})
	showQueryCheck := widget.NewCheck("show query", func(b bool) {})
	var tables = widget.NewSelect([]string{""}, func(s string) {
		result.SetText("")
		if !page.Disabled() {
			page.SetText("1")
		}
		if submit.Disabled() {
			submit.Enable()
		}
		getRows()
	})
	tables.PlaceHolder = "(table)"
	scopeEntry := widget.NewEntry()
	advancedCheck := &widget.Check{}
	contract := widget.NewSelect(TableIndex.List(), func(s string) {
		scopeEntry.SetText(s)
		if advancedCheck.Disabled() {
			advancedCheck.Enable()
		}
		t := TableIndex.Get(s)
		if len(t) == 0 {
			tables.Options = make([]string, 0)
			return
		}
		tables.Options = t
		tables.SetSelected(t[0])
	})
	contract.PlaceHolder = "(account)"
	next := &widget.Button{}
	next = widget.NewButtonWithIcon("next", theme.NavigateNextIcon(), func() {
		p, e := strconv.Atoi(page.Text)
		if e != nil {
			page.SetText("1")
		} else {
			page.SetText(strconv.Itoa(p + 1))
		}
		getRows()
	})
	next.Disable()
	previous := widget.NewButtonWithIcon("previous", theme.NavigateBackIcon(), func() {
		p, e := strconv.Atoi(page.Text)
		if e != nil {
			page.SetText("1")
		} else {
			page.SetText(strconv.Itoa(p - 1))
		}
		getRows()
	})
	previous.Disable()

	indexLabel := widget.NewLabel("index ")
	indexLabel.Hide()
	indexEntry := widget.NewEntry()
	indexEntry.SetText("1")
	indexEntry.Hide()
	scopeLabel := widget.NewLabel("scope ")
	scopeLabel.Hide()
	//scopeEntry := widget.NewEntry() // moved above contract select
	scopeEntry.SetText("")
	scopeEntry.Hide()
	typeSelect := widget.NewSelect(
		[]string{
			"name",
			"i64",
			"i128",
			"i256",
			"float64",
			"float128",
			"ripemd160",
			"sha256",
		},
		func(s string) {},
	)
	typeSelect.PlaceHolder = "(key type)"
	typeSelect.Hide()
	lowerLabel := widget.NewLabel("lower bound")
	lowerLabel.Hide()
	lowerValueEntry := widget.NewEntry()
	lowerValueEntry.SetPlaceHolder("lower bound")
	lowerValueEntry.Hide()
	upperLabel := widget.NewLabel("upper bound")
	upperLabel.Hide()
	upperValueEntry := widget.NewEntry()
	upperValueEntry.SetPlaceHolder("upper bound")
	upperValueEntry.Hide()
	lowerValueEntry.OnChanged = func(s string) {
		upperValueEntry.SetText(s)
		upperValueEntry.Refresh()
	}
	transformSelect := widget.NewSelect(
		[]string{
			"none",
			"name -> i64",
			"checksum256",
			"hash",
		},
		func(s string) {},
	)
	transformSelect.PlaceHolder = "(transform)"
	transformSelect.Hide()
	reverseCheck := widget.NewCheck("reverse", func(bool) {})
	reverseCheck.Hide()
	var lastNext, lastPrev bool
	var lastPage string
	advancedCheck = widget.NewCheck("Advanced", func(b bool) {
		if b {
			lastNext = next.Disabled()
			lastPrev = previous.Disabled()
			lastPage = page.Text
			page.SetText("-")
			scopeLabel.Show()
			scopeEntry.Show()
			indexLabel.Show()
			indexEntry.Show()
			typeSelect.Show()
			lowerLabel.Show()
			lowerValueEntry.Show()
			upperLabel.Show()
			upperValueEntry.Show()
			transformSelect.Show()
			reverseCheck.Show()
			page.Disable()
			previous.Disable()
			next.Disable()
			return
		}
		scopeLabel.Hide()
		scopeEntry.Hide()
		indexLabel.Hide()
		indexEntry.Hide()
		typeSelect.Hide()
		lowerLabel.Hide()
		lowerValueEntry.Hide()
		upperLabel.Hide()
		upperValueEntry.Hide()
		transformSelect.Hide()
		reverseCheck.Hide()
		page.SetText(lastPage)
		page.Enable()
		if !lastPrev {
			previous.Enable()
		}
		if !lastNext {
			next.Enable()
		}
	})
	advancedCheck.Disable()

	browseLayout := widget.NewVBox(
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(200, 35)),
			widget.NewHBox(
				widget.NewLabel("                              "),
				advancedCheck,
				scopeLabel,
				scopeEntry,
				indexLabel,
				indexEntry,
				typeSelect,
				lowerLabel,
				lowerValueEntry,
				upperLabel,
				upperValueEntry,
				transformSelect,
				reverseCheck,
			),
		),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(200, 35)),
			widget.NewHBox(
				widget.NewLabel("                              "),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(180, contract.MinSize().Height)), contract),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(180, tables.MinSize().Height)), tables),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(previous.MinSize()), previous),
				widget.NewLabel("page"),
				page,
				widget.NewLabel("limit"),
				rowsPerPage,
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(next.MinSize()), next),
				showQueryCheck,
				widget.NewLabel("  "),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(submit.MinSize()), submit),
			),
		),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), int(math.Round(float64(H)*.62)))),
			widget.NewScrollContainer(
				widget.NewVBox(
					result,
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{
						Width:  20,
						Height: 100,
					}), layout.NewSpacer()),
				)),
		))
	getRows = func() {
		if contract.Selected == "" || tables.Selected == "" || page.Text == "" {
			result.SetText("Invalid table params")
			return
		}
		proposedPage, e := strconv.ParseUint(page.Text, 10, 32)
		if e != nil {
			page.SetText("1")
			proposedPage = 1
		}
		var curl string
		var out *string
		var more bool
		switch advancedCheck.Checked {
		case true:
			// (max uint32, scope string, contract string, table string, index string, keyType string, lower string, upper string, transform string, api *fio.API
			out, curl, _ = QueryTableAdvanced(getRowsPerPage(), scopeEntry.Text, contract.Selected, tables.Selected, indexEntry.Text, typeSelect.Selected, lowerValueEntry.Text, upperValueEntry.Text, transformSelect.Selected, reverseCheck.Checked, api)
		case false:
			out, curl, more = QueryTable((uint32(proposedPage)-1)*getRowsPerPage(), getRowsPerPage(), contract.Selected, tables.Selected, api)
			if out == nil {
				return
			}
			if !more && !next.Disabled() {
				next.Disable()
			} else if more && next.Disabled() {
				next.Enable()
			}
			if proposedPage < 2 && !previous.Disabled() {
				previous.Disable()
			} else if proposedPage >= 2 && previous.Disabled() {
				previous.Enable()
			}
			if (!next.Disabled() || !previous.Disabled()) && page.Disabled() {
				page.Enable()
			} else if next.Disabled() && previous.Disabled() && !page.Disabled() {
				page.Disable()
			}
		}
		result.SetText("")
		var txt string
		if out != nil {
			if showQueryCheck.Checked {
				txt = `
Query:

` + curl + `

Result

` + *out

			} else {
				txt = *out
			}
			func(s string) {
				result.OnChanged = func(string) {
					result.SetText(s)
				}
			}(txt) // deref
			result.SetText(txt)
		}
	}
	TableIndex.SetCreated(true)
	return browseLayout, true
}

func QueryTable(offset uint32, max uint32, contract string, table string, api *fio.API) (out *string, query string, more bool) {
	gtr := eos.GetTableRowsRequest{
		Code:       contract,
		Scope:      contract,
		Table:      table,
		LowerBound: strconv.Itoa(int(offset)),
		Limit:      max,
		JSON:       true,
	}
	qs, _ := json.MarshalIndent(gtr, "", "  ")
	query = string(qs)
	resp, err := api.GetTableRows(gtr)
	var o string
	if err != nil {
		o = err.Error()
		return &o, query, more
	}
	more = resp.More
	j, err := json.MarshalIndent(resp.Rows, "", "  ")
	if err != nil {
		o = err.Error()
		return &o, query, more
	}
	o = string(j)
	return &o, query, more
}

func QueryTableAdvanced(max uint32, scope string, contract string, table string, index string, keyType string, lower string, upper string, transform string, reverse bool, api *fio.API) (out *string, query string, more bool) {
	if keyType == "(key type)" {
		keyType = "name"
	}
	switch transform {
	case "name -> i64":
		u, err := eos.StringToName(upper)
		if err != nil {
			e := err.Error()
			out = &e
			return
		}
		l, err := eos.StringToName(lower)
		if err != nil {
			e := err.Error()
			out = &e
			return
		}
		upper = fmt.Sprintf("%d", u)
		lower = fmt.Sprintf("%d", l)
	case "checksum256":
		h := sha256.New()
		_, err := h.Write([]byte(upper))
		if err != nil {
			e := err.Error()
			out = &e
			return
		}
		ub := h.Sum(nil)
		upper = hex.EncodeToString(ub)
		h.Reset()
		h.Write([]byte(lower))
		lb := h.Sum(nil)
		lower = hex.EncodeToString(lb)
	case "hash":
		upper = FioDomainNameHash(upper)
		lower = FioDomainNameHash(lower)
	}
	gtr := fio.GetTableRowsOrderRequest{
		Code:       contract,
		Scope:      scope,
		Table:      table,
		LowerBound: lower,
		UpperBound: upper,
		Limit:      max,
		KeyType:    keyType,
		Index:      index,
		JSON:       true,
		Reverse:    reverse,
	}
	qs, _ := json.MarshalIndent(gtr, "", "  ")
	query = string(qs)
	resp, err := api.GetTableRowsOrder(gtr)
	var o string
	if err != nil {
		o = err.Error()
		return &o, query, more
	}
	more = resp.More
	j, err := json.MarshalIndent(resp.Rows, "", "  ")
	if err != nil {
		o = err.Error()
		return &o, query, more
	}
	o = string(j)
	return &o, query, more
}
