package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/BurntSushi/toml"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	M30W struct {
		Homeserver string `toml:"homeserver"`
		Username   string `toml:"username"`
		Password   string `toml:"password"`
		Database   string `toml:"database"`
	} `toml:"m30w"`
}

type Subscription struct {
	gorm.Model
	RoomID string `gorm:"unique"`
}

var (
	config Config
	m30w   *mautrix.Client
	db     *gorm.DB
)

func init() {
	// Load configuration
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Config file loaded.")

	// Start database shenanigans
	database, err := gorm.Open(sqlite.Open(config.M30W.Database), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	db = database

	// Create database tables if then don't exist already
	db.AutoMigrate(&Subscription{})
}

func main() {
	// Sign in to the homeserver
	client, err := mautrix.NewClient(config.M30W.Homeserver, "", "")
	if err != nil {
		log.Fatal(err)
	}

	m30w = client

	syncer := m30w.Syncer.(*mautrix.DefaultSyncer)

	// When the bot is invited, it will automatically join the room
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		if evt.GetStateKey() == m30w.UserID.String() && evt.Content.AsMember().Membership == event.MembershipInvite {
			_, err := m30w.JoinRoomByID(ctx, evt.RoomID)
			if err == nil {
				// Add the subscription to the database
				db.Create(&Subscription{RoomID: evt.RoomID.String()})
				log.Println("Joined room after invite:", evt.RoomID.String(), "by", evt.Sender.String())
			} else {
				log.Println("Failed to joing room after invite: ", evt.RoomID.String(), "by", evt.Sender.String())
			}
		}
	})

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

	// Waitgroup and contexts for syncing the matrix client
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

	// Timer for sending out the cat images
	timerCtx, cancelTimer := context.WithCancel(context.Background())
	var timerStopWait sync.WaitGroup
	timerStopWait.Add(1)

	go func() {
		defer timerStopWait.Done()

		// Calculate time until next hour and do nothing
		now := time.Now()
		nextHour := now.Truncate(time.Minute).Add(time.Minute)
		timeUntilNextHour := time.Until(nextHour)
		time.Sleep(timeUntilNextHour)

		sendTheCats()

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sendTheCats()
			case <-timerCtx.Done():
				return
			}
		}
	}()

	// Wait until CTRL+C is pressed
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	<-c
	log.Println("Shutting down...")
	cancelTimer()
	timerStopWait.Wait()
	cancelSync()
	syncStopWait.Wait()
	err = cryptoHelper.Close()
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}

func sendTheCats() {
	// Get all the rooms that are subscribed to
	var subscriptions []Subscription
	result := db.Find(&subscriptions)
	if result.Error != nil {
		log.Fatal(result.Error)
	}

	// Send the cat images to all the subscribed rooms
	for _, subscription := range subscriptions {
		m30w.SendText(context.TODO(), id.RoomID(subscription.RoomID), "Cat time!")
	}
}
