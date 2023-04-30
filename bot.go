package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/json"
	"github.com/disgoorg/log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

var (
	tokenRegex   = regexp.MustCompile(`[A-Za-z\d]{24}\.[\w-]{6}\.[\w-]{27}`)
	gistApiURL   = "https://api.github.com/gists"
	gistApiToken = os.Getenv("GIST_API_TOKEN")
)

func main() {
	log.SetLevel(log.LevelInfo)
	log.Info("starting the bot...")
	log.Info("disgo version: ", disgo.Version)

	client, err := disgo.New(os.Getenv("TOKEN_INVALIDATOR_TOKEN"),
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentGuildMessages, gateway.IntentMessageContent),
			gateway.WithPresenceOpts(gateway.WithWatchingActivity("tokens"))),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagsNone)),
		bot.WithEventListeners(&events.ListenerAdapter{
			OnGuildMessageCreate: onMessage,
		}))
	if err != nil {
		log.Fatal("error while building disgo instance: ", err)
	}

	defer client.Close(context.TODO())

	if client.OpenGateway(context.TODO()) != nil {
		log.Fatal("error while connecting to the gateway: ", err)
	}

	log.Info("token invalidator bot is now running.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-s
}

func onMessage(event *events.GuildMessageCreate) {
	content := event.Message.Content
	matches := tokenRegex.FindAllString(content, -1)
	if matches != nil {
		body, err := json.Marshal(&TokenPayload{
			Description: "Token Invalidator bot by cane#8081.",
			Public:      true,
			Files: Files{
				Tokens{
					Content: strings.Join(matches, "\n"),
				},
			},
		})
		if err != nil {
			log.Error("there was an error while marshalling a token payload: ", err)
			return
		}
		r, err := http.NewRequest(http.MethodPost, gistApiURL, bytes.NewBuffer(body))
		if err != nil {
			log.Error("there was an error while creating a new request: ", err)
			return
		}
		r.Header.Add("Authorization", gistApiToken)
		r.Header.Add("Content-Type", "application/vnd.github.v3+json")
		r.Header.Add("User-Agent", "Token Invalidator bot")

		rest := event.Client().Rest()
		rs, err := rest.HTTPClient().Do(r)
		if err != nil {
			log.Error("there was an error while running a request: ", err)
			return
		}
		defer rs.Body.Close()
		var response GistResponse
		if err = json.NewDecoder(rs.Body).Decode(&response); err != nil {
			log.Errorf("there was an error while decoding the response (%d): ", rs.StatusCode, err)
			return
		}
		_, err = rest.CreateMessage(event.ChannelID, discord.MessageCreate{
			Content: fmt.Sprintf("Tokens have been detected and sent to <%s> to be invalidated.", response.URL),
			MessageReference: &discord.MessageReference{
				MessageID:       json.Ptr(event.MessageID),
				GuildID:         json.Ptr(event.GuildID),
				FailIfNotExists: false,
			},
		})
		if err != nil {
			log.Error("there was an error while creating a message: ", err)
		}
	}
}

type TokenPayload struct {
	Description string `json:"description"`
	Public      bool   `json:"public"`
	Files       Files  `json:"files"`
}

type Files struct {
	Tokens Tokens `json:"tokens.txt"`
}

type Tokens struct {
	Content string `json:"content"`
}

type GistResponse struct {
	URL string `json:"html_url"`
}
