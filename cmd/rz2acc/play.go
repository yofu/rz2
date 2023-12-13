package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/google/subcommands"
	"github.com/yofu/rz2"
)

type playCmd struct {
	directory string
}

func (*playCmd) Name() string{
	return "play"
}

func (*playCmd) Synopsis() string{
	return "play dat file"
}

func (*playCmd) Usage() string {
	return "play [-dir] <filename>"
}

func (p *playCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&p.directory, "dir", "", "directory")
}

func (p *playCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	c := make(chan rz2.ServerRecord)
	for _, fn := range f.Args() {
		go rz2.ReadServerRecordChan(filepath.Join(p.directory, fn), c)
	}
	for {
		select {
		case rec, more := <-c:
			if !more {
				return subcommands.ExitSuccess
			}
			lis := strings.Split(rec.Topic, "/")
			switch lis[2] {
			case "acc02":
				status, err := accStats(rec.Content)
				if err != nil {
					log.Printf("[play] %v\n", err)
					return subcommands.ExitFailure
				}
				fmt.Printf("%s, %s\n", rec.Topic, status)
			default:
			}
		}
	}
}
