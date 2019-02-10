package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env"
	"github.com/monzo/typhon"
	monzo "github.com/tjvr/go-monzo"
)

type config struct {
	MonzoClientID     string `env:"MONZO_CLIENT_ID,required"`
	MonzoClientSecret string `env:"MONZO_CLIENT_SECRET,required"`

	// TODO: Move these into the login process somehow, then we can serve multiple users!
	MonthlyIncome       float32 `env:"MONTHLY_INCOME,required"`
	MonthlyOutgoings    float32 `env:"MONTHLY_OUTGOINGS,required"`
	MonthlyWorkingHours float32 `env:"MONTHLY_WORKING_HOURS" envDefault:"150"`

	FeedURL      string `env:"FEED_URL" envDefault:"https://github.com/RJacksonm1/time-is-money"`
	FeedImageURL string `env:"FEED_IMAGE_URL" envDefault:"https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/240/apple/155/alarm-clock_23f0.png"`

	PublicBaseURL string `env:"PUBLIC_BASE_URL,required"`
	Port          int    `env:"PORT" envDefault:"8000"`
}

var cfg config
var auth monzo.Authenticator

// Map of Monzo account IDs to Monzo access tokens!
// If we ever restart, everybody will have to reauth. Balls.
var accountAccessTokens map[string]string

func init() {
	cfg = config{}
	err := env.Parse(&cfg)
	if err != nil {
		panic(err)
	}

	auth = *monzo.NewAuthenticator(
		cfg.MonzoClientID,
		cfg.MonzoClientSecret,
		fmt.Sprintf("%s/register", cfg.PublicBaseURL),
	)

	accountAccessTokens = make(map[string]string)
}

func webhook(req typhon.Request) typhon.Response {
	res := typhon.NewResponse(req)
	w := res.Writer()

	defer req.Body.Close()
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Bad request, bad boy", http.StatusBadRequest)
		return res
	}

	// Log all incoming data! Useful for debuggering
	log.Print(string(body))

	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		log.Print("Couldn't unwrap the JSON")
		http.Error(w, "Bad request, bad boy", http.StatusBadRequest)
		return res
	}

	// We don't give a moneys unless its a new trans
	if ev.Type != "transaction.created" {
		log.Print("Not a transaction created; skipping")
		return req.Response("OK")
	}

	var transCreatedEv TransactionCreatedEventData
	if err := json.Unmarshal(body, &transCreatedEv); err != nil {
		log.Print("Couldn't unwrap the JSON, after identifying type")
		http.Error(w, "Bad request, bad boy", http.StatusBadRequest)
		return res
	}

	// We don't care about income
	if transCreatedEv.Data.Amount > 0 {
		log.Print("Skipping incoming transaction; only care about outgoings")
		return req.Response("OK")
	}

	disposableIncome := cfg.MonthlyIncome - cfg.MonthlyOutgoings
	disposableHourlyIncome := disposableIncome / cfg.MonthlyWorkingHours
	spent := -(float32(transCreatedEv.Data.Amount) / 100)
	hoursSpent := spent / disposableHourlyIncome

	accessToken, ok := accountAccessTokens[transCreatedEv.Data.AccountID]
	if !ok {
		fmt.Print("No access token for given acconut ID", accountAccessTokens)
		http.Error(w, "We don't have any users authed for that account ID", http.StatusUnauthorized)
		return res
	}

	cl := monzo.Client{
		BaseURL:     "https://api.monzo.com",
		AccessToken: accessToken,
	}

	feedItem := monzo.FeedItem{
		AccountID: transCreatedEv.Data.AccountID,
		Type:      "basic",
		URL:       cfg.FeedURL,
		Title:     fmt.Sprintf("%.2f hours spent", hoursSpent),
		Body:      fmt.Sprintf("at %s", transCreatedEv.Data.Merchant.Name),
		ImageURL:  cfg.FeedImageURL,
	}

	err = cl.CreateFeedItem(&feedItem)
	if err != nil && err.Error() == "unauthorized.bad_access_token.expired: Access token has expired" {
		if err = auth.RefreshClient(&cl); err != nil {
			log.Print("Failed to refresh client token", err)
			http.Error(w, "Failed to refresh client token", http.StatusInternalServerError)
			return res
		}
		err = cl.CreateFeedItem(&feedItem)
	}

	if err != nil {
		log.Print("Failed to create feed item", err)
		http.Error(w, "Failed to create feed item", http.StatusInternalServerError)
		return res
	}

	return req.Response("OK")
}

func login(req typhon.Request) typhon.Response {
	res := typhon.NewResponse(req)
	w := res.Writer()
	s := auth.GetSession(w, &req.Request)
	redirectURL := auth.BeginAuthURL(s)
	log.Print(redirectURL)
	http.Redirect(w, &req.Request, redirectURL, http.StatusTemporaryRedirect)
	return res
}

func register(req typhon.Request) typhon.Response {
	res := typhon.NewResponse(req)
	w := res.Writer()
	s := auth.GetSession(w, &req.Request)

	if s.IsAuthenticated() {
		log.Print("Already registered")
		http.Redirect(w, &req.Request, "/", http.StatusTemporaryRedirect)
		return res
	}

	s = auth.Callback(w, &req.Request)
	if s == nil {
		log.Print("Callback failed")
		// TODO something went wrong
		// FIXME: Make auth.Callback log to stdout/stderr, instead of just the HTTP response; maybe fork?
		// http.Redirect(w, &req.Request, "/", http.StatusTemporaryRedirect)
		return res
	}

	// Make note of these accounts so we can marry up account webhooks to access tokens
	// And register webhooks
	accounts, err := s.Client.Accounts("uk_retail")
	if err != nil {
		log.Print("Couldn't resolve accounts", err)
		http.Error(w, "Something went super duper wrong", http.StatusInternalServerError)
		return res
	}

	for _, account := range accounts {
		accountAccessTokens[account.ID] = s.Client.AccessToken
		webhookURL := fmt.Sprintf("%s/webhook", cfg.PublicBaseURL)

		// Unregister any existing hooks for our URL
		hooks, err := s.Client.Webhooks(account.ID)
		for _, hook := range hooks {
			if hook.URL == webhookURL {
				s.Client.DeleteWebhook(hook.ID)
			}
		}

		// And register a new one
		webhook := monzo.Webhook{
			AccountID: account.ID,
			URL:       webhookURL,
		}
		_, err = s.Client.RegisterWebhook(&webhook)
		if err != nil {
			log.Print("Couldn't register webhook", err)
			http.Error(w, "Something went super duper wrong", http.StatusInternalServerError)
			return res
		}
	}

	http.Redirect(w, &req.Request, "/", http.StatusTemporaryRedirect)
	return res
}

func logout(req typhon.Request) typhon.Response {
	res := typhon.NewResponse(req)
	auth.Logout(res.Writer(), &req.Request)
	res.Encode("Logged out")
	return res
}

func index(req typhon.Request) typhon.Response {
	res := typhon.NewResponse(req)
	w := res.Writer()
	s := auth.GetSession(w, &req.Request)

	if s.IsAuthenticated() {
		return req.Response("You're logged in :) Visit /logout if you've had enough")
	}
	return req.Response("Visit /login to auth")
}

func main() {
	router := typhon.Router{}
	router.POST("/webhook", webhook)
	router.GET("/logout", logout) // Should be a POST but then I have to do HTML forms and whats not; job for another disposableHourlyIncome
	router.GET("/login", login)
	router.GET("/register", register)
	router.GET("/", index)

	svc := router.Serve().
		Filter(typhon.ErrorFilter).
		Filter(typhon.H2cFilter)
	srv, err := typhon.Listen(svc, fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		panic(err)
	}
	log.Printf("👋  Listening on %v", srv.Listener().Addr())

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	log.Printf("☠️  Shutting down")
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Stop(c)
}
