package main

import (
	"github.com/brodynelly/luuna-atlas/internal/atlas"
	"os"
)

func main() { os.Exit(atlas.NewApp().Run(os.Args[1:])) }
