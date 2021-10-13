package cryptonym

import (
	"encoding/json"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/widget"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"gopkg.in/yaml.v3"
	"math"
	"regexp"
)

func GetAbiViewer(w int, h int, api *fio.API) (tab *widget.Box, ok bool) {
	structs := widget.NewMultiLineEntry()
	actions := widget.NewMultiLineEntry()
	tables := widget.NewMultiLineEntry()
	asJson := &widget.Check{}
	scrollViews := &fyne.Container{}
	layoutStructs := &widget.TabItem{}
	layoutActions := &widget.TabItem{}
	layoutTables := &widget.TabItem{}
	r := regexp.MustCompile("(?m)^-")

	getAbi := func(s string) {
		if s == "" {
			errs.ErrChan <- "queried for empty abi"
			return
		}
		errs.ErrChan <- "getting abi for " + s
		abiOut, err := api.GetABI(eos.AccountName(s))
		if err != nil {
			errs.ErrChan <- errs.Detailed(err)
			return
		}

		var yStruct []byte
		if asJson.Checked {
			yStruct, err = json.MarshalIndent(abiOut.ABI.Structs, "", "  ")
		} else {
			yStruct, err = yaml.Marshal(abiOut.ABI.Structs)
		}
		if err != nil {
			errs.ErrChan <- err.Error()
			return
		}
		txt := r.ReplaceAllString(string(yStruct), "\n-")
		func(s string) {
			structs.SetText(s)
			structs.OnChanged = func(string) {
				structs.SetText(s)
			}
		}(txt) // deref
		structs.SetText(txt)

		var yActions []byte
		if asJson.Checked {
			yActions, err = json.MarshalIndent(abiOut.ABI.Actions, "", "  ")
		} else {
			yActions, err = yaml.Marshal(abiOut.ABI.Actions)
		}
		if err != nil {
			errs.ErrChan <- err.Error()
			return
		}
		txt = r.ReplaceAllString(string(yActions), "\n-")
		func(s string) {
			actions.OnChanged = func(string) {
				actions.SetText(s)
			}
		}(txt)
		actions.SetText(txt)

		var yTables []byte
		if asJson.Checked {
			yTables, err = json.MarshalIndent(abiOut.ABI.Tables, "", "  ")
		} else {
			yTables, err = yaml.Marshal(abiOut.ABI.Tables)
		}
		if err != nil {
			errs.ErrChan <- err.Error()
			return
		}
		txt = r.ReplaceAllString(string(yTables), "\n-")
		func(s string) {
			tables.OnChanged = func(string) {
				tables.SetText(s)
			}
		}(txt) // deref
		tables.SetText(txt)

		layoutActions.Content.Resize(actions.MinSize())
		layoutStructs.Content.Resize(structs.MinSize())
		layoutTables.Content.Resize(tables.MinSize())
		layoutActions.Content.Refresh()
		layoutStructs.Content.Refresh()
		layoutTables.Content.Refresh()
		scrollViews.Refresh()
		tables.Refresh()
		actions.Refresh()
		structs.Refresh()
	}

	abis := &widget.Select{}

	asJson = widget.NewCheck("Display Json", func(b bool) {
		structs.SetText("")
		actions.SetText("")
		tables.SetText("")
		getAbi(abis.Selected)
	})

	abis = widget.NewSelect(TableIndex.List(), func(s string) {
		structs.SetText("")
		actions.SetText("")
		tables.SetText("")
		getAbi(abis.Selected)
	})

	tabHeight := int(math.Round(float64(H) * .65))
	layoutStructs = widget.NewTabItem("Structs",
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), tabHeight)),
			fyne.NewContainerWithLayout(layout.NewMaxLayout(),
				widget.NewScrollContainer(
					structs,
				),
			),
		),
	)
	layoutActions = widget.NewTabItem("Actions",
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), tabHeight)),
			fyne.NewContainerWithLayout(layout.NewMaxLayout(),
				widget.NewScrollContainer(
					actions,
				),
			),
		),
	)
	layoutTables = widget.NewTabItem("Tables",
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), tabHeight)),
			fyne.NewContainerWithLayout(layout.NewMaxLayout(),
				widget.NewScrollContainer(
					tables,
				),
			),
		),
	)

	scrollViews = fyne.NewContainer(
		widget.NewTabContainer(
			layoutStructs,
			layoutActions,
			layoutTables,
		),
	)
	viewAbiLayout := widget.NewVBox(
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(200, 30)),
			widget.NewHBox(
				abis,
				asJson,
			),
		),
		scrollViews,
	)
	return viewAbiLayout, true
}
