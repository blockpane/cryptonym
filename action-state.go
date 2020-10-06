package cryptonym

import (
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"reflect"
	"strings"
	"sync"
)

type AbiFormItem struct {
	Contract     string
	Action       string
	Order        int
	Name         *string
	Type         *widget.Select
	SendAs       *widget.Select
	Variation    *widget.Select
	Len          *widget.Select
	Input        *widget.Entry
	Value        *interface{}
	IsSlice      bool
	SliceValue   []*interface{}
	convert      func(s interface{}) interface{}
	typeOverride string
	noJsonEscape bool // if true uses fmt, otherwise json-encodes the value ... fmt is useful for some numeric values
}

type Abi struct {
	mux sync.RWMutex

	lookUp   map[string]int
	Rows     []AbiFormItem
	Action   string
	Contract string
}

func NewAbi(length int) *Abi {
	return &Abi{
		Rows:   make([]AbiFormItem, length),
		lookUp: make(map[string]int),
	}
}

func (abi *Abi) AppendRow(myName string, account *fio.Account, form *widget.Form) {
	go func() {
		if myName == "" {
			myName = fmt.Sprintf("new_row_%d", len(abi.Rows))
		}
		if abi.Rows == nil {
			abi.Rows = make([]AbiFormItem, 0)
		}
		typeSelect := &widget.Select{}
		in := widget.NewEntry()
		num := &widget.Select{}
		inputBox := widget.NewHBox(
			widget.NewLabel("Input:"),
			in,
		)
		variation := &widget.Select{}
		sendAs := &widget.Select{}

		field := AbiFormItem{
			Contract: abi.Contract,
			Action:   abi.Action,
			Order:    len(abi.Rows),
			Name:     &myName,

			Type:      typeSelect,
			SendAs:    sendAs,
			Variation: variation,
			Len:       num,
			Input:     in,
		}

		in.OnChanged = func(s string) {
			FormState.UpdateInput(myName, in)
		}

		typeSelect = widget.NewSelect(abiSelectTypes("string"), func(s string) {
			FormState.UpdateType("string", typeSelect)
		})
		typeSelect.SetSelected("string")

		sendAs = widget.NewSelect(sendAsSelectTypes, func(send string) {
			if !strings.Contains(send, "form value") {
				inputBox.Hide()
			} else {
				inputBox.Show()
			}
			var sel string
			variation.Options, sel = sendAsVariant(send)
			variation.SetSelected(sel)
			FormState.UpdateSendAs(myName, sendAs)
		})

		sendAs.SetSelected("bytes/string")
		variation = widget.NewSelect(bytesVar, func(s string) {
			showNum, numVals, sel := getLength(s)
			if showNum {
				num.Show()
			} else {
				num.Hide()
			}
			num.Options = numVals
			num.SetSelected(sel)
			FormState.UpdateLen(myName, num)
			FormState.UpdateVariation(myName, variation)
		})
		variation.SetSelected("many AAAA...")

		num = widget.NewSelect(bytesLen, func(s string) {
			FormState.UpdateLen(myName, num)
		})
		num.SetSelected("131,072")

		form.Append(*field.Name,
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

		abi.mux.Lock()
		abi.Rows = append(abi.Rows, field)
		abi.lookUp[myName] = len(abi.Rows) - 1
		abi.mux.Unlock()
		abi.UpdateInput(myName, in)
		abi.UpdateSendAs(myName, sendAs)
		abi.UpdateType(myName, typeSelect)
		abi.UpdateLen(myName, num)
	}()
}

func (abi *Abi) AddNewRowButton(name *widget.Entry, account *fio.Account, form *widget.Form) *widget.Button {
	b := &widget.Button{}
	b = widget.NewButtonWithIcon("Add Row", theme.ContentAddIcon(), func() {
		defer name.SetText("")
		abi.mux.RLock()
		if abi.Rows != nil && len(abi.Rows) > 0 {
			if abi.lookUp[name.Text] > 0 || *abi.Rows[0].Name == name.Text {
				errs.ErrChan <- "cannot add duplicate name for abi struct"
				return
				abi.mux.RUnlock()
			}
		}
		abi.mux.RUnlock()
		abi.AppendRow(name.Text, account, form)
	})
	return b
}

func (abi *Abi) UpdateValueWithConvert(index *int, value interface{}, isSlice bool, abiType string, noJsonEscape bool) {
	if len(abi.Rows) == 0 {
		return
	}
	newVal := value
	abi.mux.Lock()
	if abiType != "" {
		abi.Rows[*index].typeOverride = abiType
		if abiType == "string" {
			// reflect on our interface to see if it should be converted from a number to a quoted number:
			t := reflect.TypeOf(value)
			if strings.Contains(t.String(), "int") || strings.Contains(t.String(), "float") {
				newVal = fmt.Sprintf("%v", value)
			}
		}
	} else {
		abi.Rows[*index].typeOverride = ""
	}
	abi.mux.Unlock()
	abi.UpdateValue(index, newVal, isSlice, noJsonEscape)
}

func (abi *Abi) UpdateValue(index *int, value interface{}, isSlice bool, noJsonEscape bool) {
	if len(abi.Rows) == 0 {
		return
	}
	abi.Rows[*index].noJsonEscape = noJsonEscape
	abi.mux.Lock()
	if !isSlice {
		abi.Rows[*index].Value = &value
	} else {
		sl := make([]*interface{}, 0)
		sl = append(sl, &value)
		abi.Rows[*index].IsSlice = true
		abi.Rows[*index].SliceValue = sl
	}
	abi.mux.Unlock()
}

func (abi *Abi) Update(index *int, abiForm AbiFormItem) {
	abi.mux.Lock()
	defer abi.mux.Unlock()
	if len(abi.Rows) == 0 {
		return
	}
	if index != nil {
		abi.lookUp[*abiForm.Name] = *index
	}
	abi.Rows[abi.lookUp[*abiForm.Name]] = abiForm
}

func (abi *Abi) UpdateType(name string, t *widget.Select) {
	if len(abi.Rows) == 0 {
		return
	}
	abi.mux.Lock()
	if abi.lookUp[name] != 0 || *abi.Rows[0].Name == name {
		abi.Rows[abi.lookUp[name]].Type = t
	}
	abi.mux.Unlock()
}

func (abi *Abi) UpdateLen(name string, t *widget.Select) {
	if len(abi.Rows) == 0 {
		return
	}
	abi.mux.Lock()
	if abi.lookUp[name] != 0 || *abi.Rows[0].Name == name {
		abi.Rows[abi.lookUp[name]].Len = t
	}
	abi.mux.Unlock()
}

func (abi *Abi) UpdateVariation(name string, t *widget.Select) {
	if len(abi.Rows) == 0 {
		return
	}
	abi.mux.Lock()
	if abi.lookUp[name] != 0 || *abi.Rows[0].Name == name {
		abi.Rows[abi.lookUp[name]].Variation = t
	}
	abi.mux.Unlock()
}

func (abi *Abi) UpdateSendAs(name string, t *widget.Select) {
	if len(abi.Rows) == 0 {
		return
	}
	abi.mux.Lock()
	if abi.lookUp[name] != 0 || *abi.Rows[0].Name == name {
		abi.Rows[abi.lookUp[name]].SendAs = t
	}
	abi.mux.Unlock()
}

func (abi *Abi) UpdateInput(name string, t *widget.Entry) {
	if len(abi.Rows) == 0 {
		return
	}
	abi.mux.Lock()
	if abi.lookUp[name] != 0 || *abi.Rows[0].Name == name {
		abi.Rows[abi.lookUp[name]].Input = t
	}
	abi.mux.Unlock()
}
