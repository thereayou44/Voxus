package main

import "github.com/thereayou/discord-lite/cmd/server"

func main() {
	srv := server.NewServer()
	srv.Run()
}
