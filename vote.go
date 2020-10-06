package cryptonym

import (
	"bytes"
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
	"gopkg.in/yaml.v2"
	"image"
	"io/ioutil"
	"log"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type existVotes struct {
	Producers []string `json:"producers"`
}

type prodRow struct {
	FioAddress string `json:"fio_address"`
}

func GetCurrentVotes(actor string, api *fio.API) (votes string) {
	getVote, err := api.GetTableRows(eos.GetTableRowsRequest{
		Code:  "eosio",
		Scope: "eosio",
		Table: "voters",

		Index:      "3",
		LowerBound: actor,
		UpperBound: actor,
		Limit:      1,
		KeyType:    "name",
		JSON:       true,
	})
	if err != nil {
		return
	}
	v := make([]*existVotes, 0)
	err = json.Unmarshal(getVote.Rows, &v)
	if err != nil {
		return
	}
	if len(v) == 0 {
		return
	}
	votedFor := make([]string, 0)
	for _, row := range v[0].Producers {
		if row == "" {
			continue
		}
		gtr, err := api.GetTableRows(eos.GetTableRowsRequest{
			Code:       "eosio",
			Scope:      "eosio",
			Table:      "producers",
			LowerBound: row,
			UpperBound: row,
			KeyType:    "name",
			Index:      "4",
			JSON:       true,
		})
		if err != nil {
			continue
		}
		p := make([]*prodRow, 0)
		err = json.Unmarshal(gtr.Rows, &p)
		if err != nil {
			continue
		}
		if len(p) == 1 && p[0].FioAddress != "" {
			votedFor = append(votedFor, p[0].FioAddress)
		}
	}
	if len(votedFor) == 0 {
		return
	}
	return strings.Join(votedFor, ", ")
}

type bpInfo struct {
	CurrentVotes float64
	FioAddress   string
	Actor        string
	BpJson       *fio.BpJson
	VoteFor      bool
	OrigVoteFor  bool
	Url          string
	Img          *canvas.Image
	Top21        bool
	Tied         bool
}

var bpInfoCache = make(map[string]*bpInfo)

func getBpInfo(actor string, api *fio.API) ([]bpInfo, error) {
	bpi := make([]bpInfo, 0)
	prods, err := api.GetFioProducers()
	if err != nil {
		return bpi, err
	}
	curVotes := strings.Split(GetCurrentVotes(actor, api), ",")
	for i := range curVotes {
		curVotes[i] = strings.TrimSpace(curVotes[i])
	}
	hasVoted := func(s string) bool {
		for _, v := range curVotes {
			if s == v {
				return true
			}
		}
		return false
	}
	isTopProd := make(map[string]bool)
	sched, _ := api.GetProducerSchedule()
	for _, tp := range sched.Active.Producers {
		isTopProd[string(tp.AccountName)] = true
	}
	voteTies := make(map[string]int)
	for _, bp := range prods.Producers {
		voteTies[bp.TotalVotes] += 1
	}
	bpiMux := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(len(prods.Producers))
	// temporarily set a very aggressive timeout, otherwise this can take up to a minute.
	// sorry BPs, if your server is slow no icon and bp info will show up.
	oldTimout := api.HttpClient.Timeout
	api.HttpClient.Timeout = 2 * time.Second
	for _, bp := range prods.Producers {
		go func(bp fio.Producer) {
			defer wg.Done()

			if bp.IsActive == 0 {
				return
			}
			votes, _ := strconv.ParseFloat(bp.TotalVotes, 64)
			bpiMux.Lock()
			var tied bool
			if voteTies[bp.TotalVotes] > 1 {
				tied = true
			}

			if bpInfoCache[string(bp.FioAddress)] != nil {
				bpInfoCache[string(bp.FioAddress)].VoteFor = hasVoted(string(bp.FioAddress))
				bpInfoCache[string(bp.FioAddress)].OrigVoteFor = hasVoted(string(bp.FioAddress))
				bpInfoCache[string(bp.FioAddress)].Tied = tied
				bpInfoCache[string(bp.FioAddress)].CurrentVotes = votes
				bpInfoCache[string(bp.FioAddress)].Top21 = isTopProd[string(bp.Owner)]
				bpi = append(bpi, *bpInfoCache[string(bp.FioAddress)])
				bpiMux.Unlock()
				return
			}

			bpiMux.Unlock()
			bpj, err := api.GetBpJson(bp.Owner)
			if err != nil {
				log.Printf("could not get bp.json for %s, %s\n", bp.Owner, err.Error())
			}
			img := canvas.NewImageFromResource(theme.QuestionIcon())
			if bpj != nil && bpj.Org.Branding.Logo256 != "" {
				resp, err := api.HttpClient.Get(bpj.Org.Branding.Logo256)
				if err == nil {
					defer resp.Body.Close()
					body, err := ioutil.ReadAll(resp.Body)
					if err == nil {
						decoded, _, err := image.Decode(bytes.NewReader(body))
						if err == nil {
							img = canvas.NewImageFromImage(decoded)
						}
					}
				}
			}
			info := bpInfo{
				CurrentVotes: votes,
				FioAddress:   string(bp.FioAddress),
				Actor:        string(bp.Owner),
				Url:          bp.Url,
				BpJson:       bpj,
				VoteFor:      hasVoted(string(bp.FioAddress)),
				OrigVoteFor:  hasVoted(string(bp.FioAddress)),
				Img:          img,
				Top21:        isTopProd[string(bp.Owner)],
				Tied:         tied,
			}
			bpiMux.Lock()
			bpi = append(bpi, info)
			bpInfoCache[string(bp.FioAddress)] = &info
			bpiMux.Unlock()
		}(bp)
	}
	wg.Wait()
	api.HttpClient.Timeout = oldTimout
	sort.Slice(bpi, func(i, j int) bool {
		if bpi[i].CurrentVotes == bpi[j].CurrentVotes {
			iName, _ := eos.StringToName(bpi[i].Actor)
			jName, _ := eos.StringToName(bpi[j].Actor)
			return iName < jName
		}
		return bpi[i].CurrentVotes > bpi[j].CurrentVotes
	})
	return bpi, nil
}

var RefreshVotesChan = make(chan bool)

func VoteContent(content chan fyne.CanvasObject, refresh chan bool) {
	pp := message.NewPrinter(language.AmericanEnglish)
	r := regexp.MustCompile("(?m)^-")
	table := func() fyne.CanvasObject {
		bpi, err := getBpInfo(string(Account.Actor), Api)
		if err != nil {
			return widget.NewLabel(err.Error())
		}

		voteRowsBox := widget.NewVBox()

		origVotes := make(map[string]bool)
		curVotes := make(map[string]bool)
		voteButton := &widget.Button{}
		countLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

		myAddrs := func() []string {
			names := make([]string, 0)
			_, _, _ = Account.GetNames(Api)
			for _, n := range Account.Addresses {
				names = append(names, n.FioAddress)
			}
			return names
		}()
		addrsSelect := widget.NewSelect(myAddrs, func(s string) {
			fee := fio.GetMaxFee(`vote_producer`)
			if err == nil {
				voteButton.SetText(pp.Sprintf("Vote! %s %g", fio.FioSymbol, fee))
			}
		})
		voteButton = widget.NewButtonWithIcon("Vote!", fioassets.NewFioLogoResource(), func() {
			go func() {
				prods := make([]string, 0)
				for k, v := range curVotes {
					if v {
						prods = append(prods, k)
					}
				}
				vp := fio.NewVoteProducer(prods, Account.Actor, addrsSelect.Selected)
				var result string
				resp, err := Api.SignPushTransaction(fio.NewTransaction(
					[]*fio.Action{vp},
					Opts,
				),
					Opts.ChainID,
					fio.CompressionNone,
				)
				if err != nil {
					result = err.Error()
					errs.ErrChan <- err.Error()
				} else {
					j, err := json.MarshalIndent(resp, "", "  ")
					if err != nil {
						errs.ErrChan <- err.Error()
						return
					}
					result = string(j)
				}
				content := fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth()/2, PctHeight()-250/2)),
					widget.NewScrollContainer(
						widget.NewLabelWithStyle(result, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
					))
				dialog.ShowCustom("voteproducer result", "OK", content, Win)
				go func() {
					time.Sleep(100 * time.Millisecond)
					RefreshVotesChan <- true
				}()
			}()
		})
		refreshButton := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
			RefreshVotesChan <- true
		})

		if len(myAddrs) > 0 {
			addrsSelect.SetSelected(myAddrs[0])
		}
		performVoteBox := widget.NewHBox(
			layout.NewSpacer(),
			countLabel,
			widget.NewLabel("vote as:"),
			addrsSelect,
			voteButton,
			refreshButton,
			layout.NewSpacer(),
		)

		refreshTopChan := make(chan map[string]bool)
		go func() {
			for _, gvc := range bpi {
				if gvc.VoteFor {
					origVotes[gvc.FioAddress] = true
					curVotes[gvc.FioAddress] = true
				}
			}
			for {
				select {
				case votee := <-refreshTopChan:
					for k, v := range votee {
						curVotes[k] = v
					}
					var votes, changed int
					for k, v := range curVotes {
						if origVotes[k] != v {
							changed = changed + 1
						}
						if v {
							votes = votes + 1
						}
					}
					countLabel.SetText(fmt.Sprintf("%d of 30 votes selected", votes))
					if changed != 0 && votes <= 30 && addrsSelect.Selected != "" && addrsSelect.Selected != "(Select one)" {
						voteButton.Enable()
						continue
					}
					voteButton.Disable()
				}
			}
		}()

		moreInfoSize := widget.NewButtonWithIcon("more info", theme.InfoIcon(), func() {}).MinSize()

		for rank, bp := range bpi {
			func(rank int, bp bpInfo) {
				voteContent := fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(2))
				var isTop string
				if bp.Top21 {
					isTop = "Top 21 "
				}
				boldName := widget.NewLabelWithStyle(bp.FioAddress, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
				boldName.Hide()
				softName := widget.NewLabel(bp.FioAddress)
				//selected := canvas.NewImageFromImage(votedImg())
				selected := canvas.NewImageFromResource(theme.NavigateNextIcon())
				selected.Hide()
				voteCheck := widget.NewCheck("Vote", func(b bool) {
					bp.VoteFor = b
					refreshTopChan <- map[string]bool{bp.FioAddress: b}
					switch {
					case bp.OrigVoteFor && bp.VoteFor:
						selected.Resource = theme.NavigateNextIcon()
						selected.Show()
					case !bp.OrigVoteFor && bp.VoteFor:
						selected.Resource = theme.ContentAddIcon()
						selected.Show()
					case bp.OrigVoteFor && !bp.VoteFor:
						selected.Resource = theme.DeleteIcon()
						selected.Show()
					default:
						selected.Hide()
					}
					selected.Refresh()
					if b {
						boldName.Show()
						softName.Hide()
						return
					}
					boldName.Hide()
					softName.Show()
				})
				voteCheck.SetChecked(bp.VoteFor)
				tied := widget.NewLabelWithStyle("   ", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
				if bp.Tied && bp.CurrentVotes > 0 {
					tied.SetText(" * ")
				}
				voteContent.AddObject(widget.NewHBox(
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(40, 40)), selected),
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(60, 40)), widget.NewLabel(isTop)),
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(40, 40)), bp.Img),
					voteCheck, boldName, softName, layout.NewSpacer(),
					widget.NewLabelWithStyle(
						pp.Sprintf("%s %d voted - rank #%3d", fio.FioSymbol, int64(math.Round(bp.CurrentVotes))/1_000_000_000, rank+1),
						fyne.TextAlignTrailing,
						fyne.TextStyle{}),
					tied,
				))
				moreInfo := widget.NewButtonWithIcon("more info", theme.InfoIcon(), func() {
					var yContent string
					y, err := yaml.Marshal(bp.BpJson)
					if err != nil {
						yContent = err.Error()
					} else {
						yContent = string(y)
					}
					mi := widget.NewMultiLineEntry() //yContent, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
					txt := r.ReplaceAllString(yContent, "\n-")
					func(txt string) {
						mi.OnChanged = func(string) {
							mi.SetText(txt)
						}
					}(txt)
					mi.SetText(txt)
					content := fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth()/2, PctHeight()-250-performVoteBox.MinSize().Height/2)),
						widget.NewScrollContainer(
							mi,
						))
					dialog.ShowCustom(bp.FioAddress, "OK", content, Win)
				})
				if bp.BpJson == nil {
					moreInfo.Hide()
					bp.Img.Hide()
				}

				nameBox := widget.NewHBox()
				if !strings.HasPrefix(bp.Url, "http") && bp.Url != "" {
					bp.Url = "http://" + bp.Url
				}
				u, err := url.Parse(bp.Url)
				nameBox.Append(fyne.NewContainerWithLayout(layout.NewFixedGridLayout(moreInfoSize), moreInfo))
				if err == nil {
					nameBox.Append(widget.NewHyperlinkWithStyle(u.Host, u, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
				}
				nameBox.Append(layout.NewSpacer())
				//nameBox.Append(widget.NewLabelWithStyle("     ", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true}))
				voteContent.AddObject(nameBox)
				voteRowsBox.Append(voteContent)
			}(rank, bp)
		}
		voteRowsBox.Append(fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{
			Width:  20,
			Height: 50,
		})))
		return widget.NewVBox(
			performVoteBox,
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), PctHeight()-250-performVoteBox.MinSize().Height)),
				widget.NewGroupWithScroller("Producer Candidates", voteRowsBox),
			))
	}
	content <- widget.NewLabel("Please wait, updating producer information ....")
	content <- table()
	for {
		select {
		case <-refresh:
			content <- widget.NewLabel("Please wait, updating producer information ....")
			content <- table()
		}
	}
}
