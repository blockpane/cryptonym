package cryptonym

import (
	"fmt"
	errs "github.com/blockpane/cryptonym/errLog"
	"log"
	"os"
	"syscall"
)

func startErrLog() {
	d, e := os.UserConfigDir()
	if e != nil {
		log.Println(e)
		return
	}
	errLog, e := os.OpenFile(fmt.Sprintf("%s%c%s%cerror.log", d, os.PathSeparator, settingsDir, os.PathSeparator), os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_SYNC, 0600)
	if e != nil {
		log.Println(e)
		return
	}
	e = syscall.Dup2(int(errLog.Fd()), 1)
	if e != nil {
		log.Println(e)
		return
	}
	e = syscall.Dup2(int(errLog.Fd()), 2)
	if e != nil {
		log.Println(e)
		return
	}
	errs.ErrChan <- fmt.Sprintf("Writing session log to: %s%c%s%cerror.log", d, os.PathSeparator, settingsDir, os.PathSeparator)
}
