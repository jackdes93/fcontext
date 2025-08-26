package fcontext

import (
	"flag"
	"fmt"
)

type AppFlatSet struct {
	fs *flag.FlagSet
}

func newFlagSet(name string, fs *flag.FlagSet) *AppFlatSet {
	if fs == nil {
		fs = flag.CommandLine
	}
	return &AppFlatSet{fs: fs}
}

func (a *AppFlatSet) Parse(args []string) {
	_ = a.fs.Parse(args)
}

func (a *AppFlatSet) GetSampleEnvs() {
	fmt.Println("# Sample ENVs")
	fmt.Println("APP_ENV=dev          # dev|stg|prd")
	fmt.Println("ENV_FILE=.env          # đường dẫn .env")
}
