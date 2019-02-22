package main

import (
	"context"
	"database/sql"
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
	_ "github.com/mattn/go-sqlite3"
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
	DataDirectory string `env:"DATA_DIR" envDefault:"."`
}

var db *sql.DB
var cfg config
var auth monzo.Authenticator

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

	err = db.Ping()
	if err != nil {
		fmt.Print("Failed to ping database")
		http.Error(w, "Failed to ping database", http.StatusInternalServerError)
		return res
	}

	accountID := transCreatedEv.Data.AccountID
	var accessToken, refreshToken string
	err = db.QueryRow("select access_token, refresh_token from account_tokens where account_id = ?", accountID).Scan(&accessToken, &refreshToken)
	if err != nil {
		fmt.Printf("No access token for account ID %s; %s", accountID, err)
		http.Error(w, fmt.Sprintf("No access token for account ID %s; %s", accountID, err), http.StatusUnauthorized)
		return res
	}

	cl := monzo.Client{
		BaseURL:      "https://api.monzo.com",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}

	feedItem := monzo.FeedItem{
		AccountID: accountID,
		Type:      "basic",
		URL:       cfg.FeedURL,
		Title:     fmt.Sprintf("%.2f hours spent", hoursSpent),
		Body:      fmt.Sprintf("at %s", transCreatedEv.Data.Merchant.Name),
		ImageURL:  cfg.FeedImageURL,
	}

	err = cl.CreateFeedItem(&feedItem)
	if err != nil && err.Error() == "unauthorized.bad_access_token.expired: Access token has expired" {

		// auth.RefreshClient will send through the expired authorization header, which Monzo's API
		// gateway/middleware will block before it realises we're trying to refresh
		cl.AccessToken = ""

		err = auth.RefreshClient(&cl)
		if err != nil {
			log.Print("Failed to refresh client token: ", err)
			http.Error(w, "Failed to refresh client token", http.StatusInternalServerError)
			return res
		}

		// Update access tokens now we've refreshed them
		stmt, _ := db.Prepare("REPLACE INTO account_tokens (account_id, access_token, refresh_token) VALUES(?, ?, ?)")
		_, err = stmt.Exec(accountID, cl.AccessToken, cl.RefreshToken)
		if err != nil {
			log.Print("Failed to save refreshed access tokens", err)
			http.Error(w, "Something went super duper wrong", http.StatusInternalServerError)
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
		// Save this user's access tokens against each of their accounts
		stmt, err := db.Prepare("REPLACE INTO account_tokens (account_id, access_token, refresh_token) VALUES(?, ?, ?)")
		if err != nil {
			log.Print("Failed to prepare query to save access tokens", err)
			http.Error(w, "Something went super duper wrong", http.StatusInternalServerError)
			return res
		}
		_, err = stmt.Exec(account.ID, s.Client.AccessToken, s.Client.RefreshToken)
		if err != nil {
			log.Print("Failed to save access tokens", err)
			http.Error(w, "Something went super duper wrong", http.StatusInternalServerError)
			return res
		}

		// Sort out webhooks
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
	var err error
	db, err = sql.Open("sqlite3", fmt.Sprintf("%s/time-is-money.db", cfg.DataDirectory))
	if err != nil {
		panic(err)
	}

	// And make sure we've got somewhere to store access tokens
	statement, err := db.Prepare(
		`CREATE TABLE IF NOT EXISTS account_tokens (
			account_id varchar(255) PRIMARY KEY,
			access_token varchar(255),
			refresh_token varchar(255)
		)`)
	if err != nil {
		panic(err)
	}
	statement.Exec()

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
