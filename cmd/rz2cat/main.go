package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/yofu/rz2"
)

func ReadData(datdir string, fns ...string) error {
	for _, fn := range fns {
		records, err := rz2.ReadServerRecord(filepath.Join(datdir, fn))
		if err != nil {
			return err
		}
		for _, rec := range records {
			fmt.Println(rec.Topic, len(rec.Content))
		}
	}
	return nil
}

func main() {
	datdir := flag.String("dir", ".", "dat directory")
	flag.Parse()
	datfn := flag.Args()
	err := ReadData(*datdir, datfn...)
	if err != nil {
		log.Fatal(err)
	}
}
