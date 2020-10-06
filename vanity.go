package cryptonym

import (
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos/ecc"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"strings"
	"sync"
	"time"
)

func vanityKey(o *vanityOptions, quit chan bool) (*fio.Account, error) {
	account := &fio.Account{}
	var hit bool
	errs.ErrChan <- "vanity search starting for " + o.word
	found := func(k *key) {
		errs.ErrChan <- fmt.Sprintf("Vanity generator found a match: %s %s", k.actor, k.pub)
	}

	statsChan := make(chan bool)
	summary := make(chan bool)
	go func() {
		pp := message.NewPrinter(language.AmericanEnglish)
		t := time.NewTicker(time.Minute / 2)
		var counter uint64
		var total uint64
		for {
			select {
			case <-summary:
				errs.ErrChan <- pp.Sprintf("... tried %d keys", total)
			case <-t.C:
				if hit {
					return
				}
				errs.ErrChan <- pp.Sprintf("Key search rate: %d KPS, tried %d so far", counter/30, total)
				counter = 0
			case <-statsChan:
				counter += 1
				total += 1
			}
		}
	}()

	finishChan := make(chan bool)
	keyChan := make(chan *key)
	var err error
	go func() {
		for {
			select {
			case <-finishChan:
				summary <- true
				return
			case <-quit:
				hit = true
			case k := <-keyChan:
				if hit {
					continue
				}
				switch o.anywhere {
				case false:
					if o.actor {
						if strings.HasPrefix(k.actor, o.word) {
							hit = true
						}
					}
					if o.pub {
						if strings.HasPrefix(strings.ToLower(k.pub[4:]), strings.ToLower(o.word)) {
							hit = true
						}
					}
				case true:
					if o.actor {
						if strings.Contains(k.actor, o.word) {
							hit = true
						}
					}
					if o.pub {
						if strings.Contains(strings.ToLower(k.pub[4:]), strings.ToLower(o.word)) {
							hit = true
						}
					}
				}
				if hit {
					found(k)
					account, err = fio.NewAccountFromWif(k.priv)
					if err != nil {
						errs.ErrChan <- err.Error()
					}
				}
			}
		}
	}()

	wg := sync.WaitGroup{}
	for i := 0; i < o.threads; i++ {
		wg.Add(1)
		go func() {
			for {
				if hit {
					wg.Done()
					return
				}
				keyChan <- newRandomAccount()
				statsChan <- true
			}
		}()
	}
	wg.Wait()
	finishChan <- true
	errs.ErrChan <- "vanity key generator done"
	return account, err
}

type key struct {
	actor string
	pub   string
	priv  string
}

func newRandomAccount() *key {
	priv, _ := ecc.NewRandomPrivateKey()
	pub := "FIO" + priv.PublicKey().String()[3:]
	actor, _ := fio.ActorFromPub(pub)
	return &key{
		actor: string(actor),
		pub:   pub,
		priv:  priv.String(),
	}
}

type vanityOptions struct {
	anywhere bool
	word     string
	actor    bool
	pub      bool
	threads  int
}
