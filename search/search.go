/*
 * Ferret
 * Copyright (c) 2016 Yieldbot, Inc.
 * For the full copyright and license information, please view the LICENSE.txt file.
 */

// Package search provides search interface and functionality
package search

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	prov "github.com/yieldbot/ferret/providers"
	"github.com/yieldbot/gocli"
	"golang.org/x/net/context"
)

var (
	// Logger is the logger
	Logger *log.Logger

	goCommand     = "open"
	searchTimeout = "5000ms"
	providers     = make(map[string]Provider)
)

func init() {
	Logger = log.New(os.Stderr, "", log.LstdFlags)

	if e := os.Getenv("FERRET_GOTO_CMD"); e != "" {
		goCommand = e
	}
	if e := os.Getenv("FERRET_SEARCH_TIMEOUT"); e != "" {
		searchTimeout = e
	}

	prov.Register(Register)
}

// Provider represents a provider
type Provider struct {
	Name    string
	Title   string
	Enabled bool
	Noui    bool
	Searcher
}

// Searcher is the interface that must be implemented by a search provider
type Searcher interface {
	// Search makes a search
	Search(ctx context.Context, args map[string]interface{}) ([]map[string]interface{}, error)
}

// Register registers a search provider
func Register(provider interface{}) error {

	// Init provider
	p, ok := provider.(Searcher)
	if !ok {
		return errors.New("invalid provider")
	}

	// Determine the provider info
	var name, title string
	var enabled, noui bool
	// Get the value of the provider
	v := reflect.Indirect(reflect.ValueOf(p))
	// Iterate the provider fields
	for i := 0; i < v.NumField(); i++ {
		fn := v.Type().Field(i).Name
		ft := v.Field(i).Type().Name()
		if ft == "string" {
			if fn == "name" {
				name = v.Field(i).String()
			} else if fn == "title" {
				title = v.Field(i).String()
			}
		} else if ft == "bool" {
			if fn == "enabled" {
				enabled = v.Field(i).Bool()
			} else if fn == "noui" {
				noui = v.Field(i).Bool()
			}
		}
	}
	if name == "" {
		return errors.New("invalid provider name")
	}
	if title == "" {
		title = name
	}

	// Check and init the provider
	if _, ok := providers[name]; ok {
		return errors.New("search provider " + name + " is already registered")
	}
	np := Provider{
		Name:     name,
		Title:    title,
		Enabled:  enabled,
		Noui:     noui,
		Searcher: p,
	}
	providers[name] = np

	return nil
}

// Providers returns a sorted list of the names of the providers
func Providers() []string {
	var list = []string{}
	for name := range providers {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

// ProviderByName gets a provider by the given name
func ProviderByName(name string) (Provider, error) {
	p, ok := providers[name]
	if !ok {
		return p, errors.New("provider " + name + " couldn't be found")
	}
	return p, nil
}

// Do makes a search query by the given context and query
func Do(ctx context.Context, query Query) (Query, error) {

	// Provider
	p, ok := providers[query.Provider]
	if !ok {
		query.HTTPStatus = http.StatusBadRequest
		return query, fmt.Errorf("invalid search provider. Possible search providers are %s", Providers())
	}

	// Keyword
	if query.Keyword == "" {
		query.HTTPStatus = http.StatusBadRequest
		return query, errors.New("missing keyword")
	}

	// Page
	if query.Page <= 0 {
		query.HTTPStatus = http.StatusBadRequest
		return query, errors.New("invalid page #. It should be greater than 0")
	}

	// Search
	query.Start = time.Now()
	ctx, cancel := context.WithTimeout(ctx, query.Timeout)
	defer cancel()
	sq := map[string]interface{}{"page": query.Page, "keyword": query.Keyword}
	sr, err := p.Search(ctx, sq)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			query.HTTPStatus = http.StatusGatewayTimeout
			return query, errors.New("timeout")
		}
		if strings.Contains(err.Error(), "context canceled") {
			query.HTTPStatus = http.StatusInternalServerError
			return query, errors.New("canceled")
		}
		query.HTTPStatus = http.StatusInternalServerError
		return query, errors.New("failed to search due to " + err.Error())
	}
	query.Elapsed = time.Since(query.Start)
	for _, srv := range sr {
		var d string
		if _, ok := srv["Description"]; ok {
			d = srv["Description"].(string)
		}

		var t time.Time
		if _, ok := srv["Date"]; ok {
			t = srv["Date"].(time.Time)
		}

		query.Results = append(query.Results, Result{
			Link:        srv["Link"].(string),
			Title:       srv["Title"].(string),
			Description: d,
			Date:        t,
		})
	}

	// Goto
	if query.Goto != 0 {
		if query.Goto < 0 || query.Goto > len(query.Results) {
			return query, fmt.Errorf("invalid result # to go. It should be between 1 and %d", len(query.Results))
		}
		link := query.Results[query.Goto-1].Link
		if _, err = exec.Command(goCommand, link).Output(); err != nil {
			return query, fmt.Errorf("failed to go to %s due to %s. Check FERRET_GOTO_CMD environment variable", link, err.Error())
		}
		return query, nil
	}

	return query, nil
}

// PrintResults prints the given search results
func PrintResults(query Query, err error) {
	if err != nil {
		Logger.Fatal(err)
	}

	if query.Goto == 0 {
		t := gocli.Table{}
		t.AddRow(1, "#", "TITLE")
		for i, v := range query.Results {
			ts := ""
			if !v.Date.IsZero() {
				ts = fmt.Sprintf(" (%d-%02d-%02d)", v.Date.Year(), v.Date.Month(), v.Date.Day())
			}
			t.AddRow(i+2, fmt.Sprintf("%d", i+1), fmt.Sprintf("%s%s", v.Title, ts))
		}
		t.PrintData()
		fmt.Printf("\n%dms\n", int64(query.Elapsed/time.Millisecond))
	}
}

// ParsePage parses page from a given string
func ParsePage(page string) int {
	p := 1
	if page != "" {
		i, err := strconv.Atoi(page)
		if err == nil && i > 0 {
			p = i
		}
	}
	return p
}

// ParseGoto parses goto from a given string
func ParseGoto(gt string) int {
	g := 0
	if gt != "" {
		i, err := strconv.Atoi(gt)
		if err == nil && i > 0 {
			g = i
		}
	}
	return g
}

// ParseTimeout parses timeout from a given string
func ParseTimeout(timeout string) time.Duration {
	t := 5000 * time.Millisecond
	if timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err == nil {
			t = d
		}
	} else {
		d, err := time.ParseDuration(searchTimeout)
		if err == nil {
			t = d
		}
	}
	return t
}
