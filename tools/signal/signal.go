package signal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var onlyOneSignalHandler = make(chan struct{})

func SetupSignal(fns ...func()) context.Context {
	close(onlyOneSignalHandler)
	sigch := make(chan os.Signal, 2)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigch
		time.AfterFunc(time.Second*30, func() {
			os.Exit(0)
		})
		for _, fn := range fns {
			fn()
		}
		cancel()

		<-sigch
		os.Exit(0)
	}()
	return ctx
}
