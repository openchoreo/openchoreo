// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Command demo-app serves both versions of every progressive delivery
// sample. MODE picks the app:
//
//   - trips: the trips service; ENGINE and LATENCY_MS tell the Python
//     original from the Go rewrite (canary and mirror samples).
//   - checkout: the checkout service, charging VAT at the seller's or the
//     buyer's country rate (blue/green demo).
//   - storefront: the cart page, with or without recommendations (A/B demo).
//
// Every response says who answered in the X-Served-By header. GET /stats
// reports how many requests the app served, which for a mirror candidate with
// no callers of its own is the copies it received.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var (
	name = env("APP_NAME", "demo-app")
	mode = env("MODE", "trips")

	// served counts the requests the app answered.
	served atomic.Int64
)

// counted counts each request.
func counted(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		next(w, r)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil {
		return def
	}
	return v
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) })
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"requests": served.Load()})
	})
	switch mode {
	case "trips":
		mux.HandleFunc("/", counted(trips))
	case "checkout":
		mux.HandleFunc("/", counted(checkout))
	case "storefront":
		mux.HandleFunc("/", counted(storefront))
	default:
		log.Fatalf("unknown MODE %q", mode)
	}
	addr := ":" + env("PORT", "8080")
	log.Printf("%s (%s) listening on %s", name, mode, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Served-By", name)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// trips lists a rider's last trips. Both versions share the API; ENGINE and
// LATENCY_MS set which one answers. FAIL_RATE (0 to 1) makes a share of
// requests fail, to show a step back.
func trips(w http.ResponseWriter, _ *http.Request) {
	time.Sleep(time.Duration(envFloat("LATENCY_MS", 15)*(0.8+0.4*rand.Float64())) * time.Millisecond)
	if rand.Float64() < envFloat("FAIL_RATE", 0) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"servedBy": name, "error": "trip store timeout"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"servedBy": name,
		"engine":   env("ENGINE", "go"),
		"trips": []map[string]any{
			{"id": "t-1001", "from": "Colombo Fort", "to": "Bambalapitiya", "fare": 640},
			{"id": "t-1002", "from": "Nugegoda", "to": "Rajagiriya", "fare": 910},
			{"id": "t-1003", "from": "Dehiwala", "to": "Colombo Fort", "fare": 780},
		},
	})
}

// sellerCountry is where the demo shop is established.
const sellerCountry = "NL"

// vatRates are standard VAT rates in 2021.
var vatRates = map[string]float64{"NL": 21, "DE": 19, "FR": 20, "IT": 22, "SE": 25, "IE": 23}

// checkout prices an order. With TAX_MODE=seller it charges the shop's own
// rate, as distance sales under the threshold did before 1 July 2021; with
// TAX_MODE=buyer it charges the buyer's country rate, as the EU One-Stop
// Shop rules require from that date.
func checkout(w http.ResponseWriter, r *http.Request) {
	country := strings.ToUpper(r.URL.Query().Get("country"))
	if _, ok := vatRates[country]; !ok {
		country = "DE"
	}
	amount := 100.0
	if v, err := strconv.ParseFloat(r.URL.Query().Get("amount"), 64); err == nil && v > 0 {
		amount = v
	}
	taxMode := env("TAX_MODE", "seller")
	rateCountry := sellerCountry
	if taxMode == "buyer" {
		rateCountry = country
	}
	rate := vatRates[rateCountry]
	writeJSON(w, http.StatusOK, map[string]any{
		"servedBy": name,
		"color":    env("COLOR", "blue"),
		"taxMode":  taxMode,
		"buyer":    country,
		"vat":      fmt.Sprintf("%.0f%% (%s)", rate, rateCountry),
		"net":      amount,
		"total":    amount * (1 + rate/100),
	})
}

// experimentCookie holds the visitor's group. The storefront assigns it once,
// 50/50, so the gateway can keep each visitor in their group.
const experimentCookie = "exp_cartrecs"

// overHTTPS reports whether the visitor's request came over HTTPS, directly
// or through a gateway that ended TLS and said so in X-Forwarded-Proto.
func overHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// storefront renders the cart. RECS=true adds "Customers also bought".
func storefront(w http.ResponseWriter, r *http.Request) {
	group := ""
	if c, err := r.Cookie(experimentCookie); err == nil {
		group = c.Value
	}
	if (group != "a" && group != "b") && r.URL.Query().Get("assigned") == "" {
		group = "a"
		if rand.IntN(2) == 1 {
			group = "b"
		}
		http.SetCookie(w, &http.Cookie{
			Name: experimentCookie, Value: group, Path: "/", MaxAge: 30 * 24 * 3600,
			// Only the gateway and this server read it. Secure whenever the
			// visitor came over HTTPS; the demo gateway also serves plain HTTP,
			// where a Secure cookie would never be sent back.
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: overHTTPS(r),
		})
		// This request reached the storefront without a group, so the gateway
		// sent it to group a's version. Ask again with the cookie, so a new
		// group b visitor sees group b's; assigned stops a client that keeps
		// no cookies from asking forever.
		q := r.URL.Query()
		q.Set("assigned", "1")
		u := *r.URL
		u.RawQuery = q.Encode()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Served-By", name)
		http.Redirect(w, r, u.RequestURI(), http.StatusFound)
		return
	}
	if group != "a" && group != "b" {
		group = "a"
	}
	recs := env("RECS", "false") == "true"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Served-By", name)
	w.Header().Set("X-Experiment-Group", group)
	fmt.Fprintf(w, "<h1>Your cart</h1><p>1 x Kindle Paperwhite, $139.99</p><p><small>served by %s, group %s</small></p>", name, group)
	if recs {
		fmt.Fprint(w, "<h2>Customers who bought this also bought</h2><ul><li>Kindle cover</li><li>USB-C charger</li></ul>")
	}
}
