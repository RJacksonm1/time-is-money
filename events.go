package main

import "time"

// Event received from the Monzo webhook
type Event struct {
	Type string `json:"type"`
}

// TransactionCreatedEventData 💸
type TransactionCreatedEventData struct {
	*Event
	Data struct {
		AccountID   string    `json:"account_id"`
		Amount      int       `json:"amount"`
		Created     time.Time `json:"created"`
		Currency    string    `json:"currency"`
		Description string    `json:"description"`
		ID          string    `json:"id"`
		Category    string    `json:"category"`
		IsLoad      bool      `json:"is_load"`
		Settled     time.Time `json:"settled"`

		Merchant struct {
			Address struct {
				Address   string  `json:"address"`
				City      string  `json:"city"`
				Country   string  `json:"country"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				Postcode  string  `json:"postcode"`
				Region    string  `json:"region"`
			} `json:"address"`
			Created  time.Time `json:"created"`
			GroupID  string    `json:"group_id"`
			ID       string    `json:"id"`
			Logo     string    `json:"logo"`
			Emoji    string    `json:"emoji"`
			Name     string    `json:"name"`
			Category string    `json:"category"`
		} `json:"merchant"`
	} `json:"data"`
}
