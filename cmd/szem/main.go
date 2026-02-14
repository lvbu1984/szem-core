package main

import (
	"log"

	"github.com/lvbu1984/szem-core/internal/api"
	"github.com/lvbu1984/szem-core/internal/lifecycle"
	"github.com/lvbu1984/szem-core/internal/storage"
)

func main() {
	store, err := lifecycle.OpenSQLite("./data/meta.db")
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	adapter := storage.NewMockAdapter()

	server := api.NewServer(store, adapter)

	log.Fatal(server.Start(":8080"))
}

