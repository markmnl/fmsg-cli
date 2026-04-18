package main

import (
	"github.com/joho/godotenv"
	"github.com/markmnl/fmsg-cli/cmd"
)

func main() {
	_ = godotenv.Load() // silently ignore if .env is absent
	cmd.Execute()
}
