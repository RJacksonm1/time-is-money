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
		Settled     string    `json:"settled"` // Empty string if not yet settled

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

// TODO:
// Turns out there's a ton more possible fields here
/*
{
  "type": "transaction.created",
  "data": {
    "id": "alalalalala",
    "created": "2019-02-07T00:23:11.401Z",
    "description": "WWW.CODECLUB.ORG.UK    CAMBRIDGE     GBR",
    "amount": -1000,
    "fees": {},
    "currency": "GBP",
    "merchant": {
      "id": "alalalalala",
      "group_id": "alalalalala",
      "created": "2018-12-06T15:31:53.347Z",
      "name": "www.codeclub.org.uk",
      "logo": "",
      "emoji": "",
      "category": "general",
      "online": true,
      "atm": false,
      "address": {
        "short_formatted": "30 Station Road, Cambridge CB1 2JH",
        "formatted": "30 Station Road, Cambridge, Lnd CB1 2JH, United Kingdom",
        "address": "30 Station Road",
        "city": "CAMBRIDGE",
        "region": "LND",
        "country": "GBR",
        "postcode": "CB1 2JH",
        "latitude": 52.1943342,
        "longitude": 0.1350405,
        "zoom_level": 4,
        "approximate": false
      },
      "updated": "2018-12-07T03:54:15.831Z",
      "metadata": {
        "created_for_transaction": "alalalalala",
        "enriched_from_settlement": "alalalalala"
      },
      "disable_feedback": false
    },
    "notes": "",
    "metadata": {
      "ledger_insertion_id": "alalalalala",
      "mastercard_approval_type": "full",
      "mastercard_auth_message_id": "alalalalala",
      "mastercard_lifecycle_id": "alalalalala",
      "mcc": "1234567890"
    },
    "labels": null,
    "account_balance": 0,
    "attachments": null,
    "international": null,
    "category": "general",
    "is_load": false,
    "settled": "",
    "local_amount": -1000,
    "local_currency": "GBP",
    "updated": "2019-02-07T00:23:11.492Z",
    "account_id": "alalalalala",
    "user_id": "alalalalala",
    "counterparty": {},
    "scheme": "mastercard",
    "dedupe_id": "alalalalala",
    "originator": false,
    "include_in_spending": true,
    "can_be_excluded_from_breakdown": true,
    "can_be_made_subscription": true,
    "can_split_the_bill": true,
    "can_add_to_tab": true
  }
}

*/
