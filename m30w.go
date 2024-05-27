package main

import (
	"context"
	"errors"
	"log"
	"sync"

	_ "github.com/mattn/go-sqlite3"

	"github.com/BurntSushi/toml"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
)

type Config struct {
	M30W struct {
		Homeserver string `toml:"homeserver"`
		Username   string `toml:"username"`
		Password   string `toml:"password"`
		Database   string `toml:"database"`
	} `toml:"m30w"`
}

var config Config

func init() {
	// Load configuration
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatal(err)
	}

	// Start databse shenanigans

}

func main() {

	// Sign in to the homeserver
	m30w, err := mautrix.NewClient(config.M30W.Homeserver, "", "")
	if err != nil {
		log.Fatal(err)
	}

	cryptoHelper, err := cryptohelper.NewCryptoHelper(m30w, []byte("m30w"), config.M30W.Database)
	if err != nil {
		log.Fatal(err)
	}
	cryptoHelper.LoginAs = &mautrix.ReqLogin{
		Type:       mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: config.M30W.Username},
		Password:   config.M30W.Password,
	}

	err = cryptoHelper.Init(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	m30w.Crypto = cryptoHelper

	log.Println("Logged in as", config.M30W.Username)

	syncCtx, cancelSync := context.WithCancel(context.Background())
	var syncStopWait sync.WaitGroup
	syncStopWait.Add(1)

	go func() {
		err = m30w.SyncWithContext(syncCtx)
		defer syncStopWait.Done()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	}()

	cancelSync()
	syncStopWait.Wait()
	err = cryptoHelper.Close()
	if err != nil {
		log.Fatal(err)
	}
}
